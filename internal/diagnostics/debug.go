package diagnostics

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
	"unicode/utf8"

	"github.com/napisani/secret_inject/internal/privatefs"
	"github.com/napisani/secret_inject/internal/profile"
)

const maxSize = 5 * 1024 * 1024

type Kind string

const (
	ProfileSelected Kind = "profile-selected"
	CacheState      Kind = "cache-state"
	ProviderState   Kind = "provider-state"
	ProviderFailed  Kind = "provider-failed"
	SourceOverwrite Kind = "source-overwrite"
	StorageProgress Kind = "storage-progress"
	ResultPrepared  Kind = "result-prepared"
	Failure         Kind = "failure"
	Unmatched       Kind = "unmatched"
	RenameSkipped   Kind = "rename-skipped"
	RenameCollision Kind = "rename-collision"
)

type Event struct {
	Kind        Kind   `json:"kind"`
	Profile     string `json:"profile,omitempty"`
	State       string `json:"state,omitempty"`
	Provider    string `json:"provider,omitempty"`
	Operation   string `json:"operation,omitempty"`
	Failure     string `json:"failure,omitempty"`
	ExitCode    *int   `json:"exit_code,omitempty"`
	Mode        string `json:"mode,omitempty"`
	Rule        string `json:"rule,omitempty"`
	Original    string `json:"original,omitempty"`
	Reason      string `json:"reason,omitempty"`
	Destination string `json:"destination,omitempty"`
	Winner      string `json:"winner,omitempty"`
	Discarded   string `json:"discarded,omitempty"`
	Count       int    `json:"count,omitempty"`
}

type Session struct {
	file     *os.File
	lockFile *os.File
	path     string
	err      error
	finished bool
}

// logError keeps filesystem causes inspectable without rendering private paths.
type logError struct {
	message string
	cause   error
}

func (e *logError) Error() string { return e.message }
func (e *logError) Unwrap() error { return e.cause }

func logFailure(message string, cause error) error {
	return &logError{message: message, cause: cause}
}

func Open() (*Session, error) {
	root := os.Getenv("XDG_STATE_HOME")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, logFailure("debug home directory unavailable", err)
		}
		root = filepath.Join(home, ".local", "state")
	}
	if !filepath.IsAbs(root) {
		return nil, errors.New("debug state directory must be absolute")
	}
	directory := filepath.Join(root, "secret_inject")
	if err := privatefs.EnsureDir(directory); err != nil {
		return nil, logFailure("private debug directory unavailable", err)
	}
	lockFile, err := privatefs.Open(filepath.Join(directory, "debug.lock"), os.O_CREATE|os.O_RDWR)
	if err != nil {
		return nil, logFailure("private debug lock unavailable", err)
	}
	if err := tryLock(lockFile); err != nil {
		return nil, logFailure("debug lock busy or unavailable", errors.Join(err, lockFile.Close()))
	}
	s := &Session{lockFile: lockFile, path: filepath.Join(directory, "debug.log")}
	s.file, err = privatefs.Open(s.path, os.O_CREATE|os.O_RDWR|os.O_APPEND)
	if err != nil {
		return nil, logFailure("private debug log unavailable", errors.Join(err, s.Finish()))
	}
	backup, err := privatefs.Open(s.path+".1", os.O_RDONLY)
	if err == nil {
		err = backup.Close()
	} else if errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	if err != nil {
		return nil, logFailure("private debug backup unavailable", errors.Join(err, s.Finish()))
	}
	return s, nil
}

func clip(value string) string {
	if utf8.RuneCountInString(value) <= 64 {
		return value
	}
	return string([]rune(value)[:64]) + "…"
}

func (s *Session) Record(event Event) {
	if s == nil || s.err != nil || s.finished {
		return
	}
	switch event.Kind {
	case ProfileSelected, CacheState, ProviderState, ProviderFailed,
		SourceOverwrite, StorageProgress, ResultPrepared, Failure,
		Unmatched, RenameSkipped, RenameCollision:
	default:
		s.err = errors.New("invalid debug event")
		return
	}
	fields := []*string{
		&event.Profile,
		&event.State,
		&event.Provider,
		&event.Operation,
		&event.Failure,
		&event.Mode,
		&event.Rule,
		&event.Original,
		&event.Reason,
		&event.Destination,
		&event.Winner,
		&event.Discarded,
	}
	for _, field := range fields {
		*field = clip(*field)
	}
	record := struct {
		Time string `json:"time"`
		Event
	}{Time: time.Now().UTC().Format(time.RFC3339Nano), Event: event}
	data, err := json.Marshal(record)
	if err != nil {
		s.err = logFailure("invalid debug record", err)
		return
	}
	if len(data) > 8191 {
		s.err = errors.New("invalid debug record")
		return
	}
	data = append(data, '\n')
	info, err := s.file.Stat()
	if err == nil && info.Size()+int64(len(data)) > maxSize {
		err = s.rotate()
	}
	if err == nil {
		_, err = s.file.Write(data)
	}
	if err != nil {
		s.err = logFailure("debug log write or rotation failed", err)
	}
}

func (s *Session) RecordSelection(events []profile.Event) {
	for _, event := range events {
		s.Record(Event{
			Kind:        Kind(event.Kind),
			Mode:        event.Mode,
			Rule:        event.Rule,
			Original:    event.Original,
			Reason:      event.Reason,
			Destination: event.Destination,
			Winner:      event.Winner,
			Discarded:   event.Discarded,
		})
	}
}

func (s *Session) rotate() error {
	if err := errors.Join(s.file.Sync(), s.file.Close()); err != nil {
		s.file = nil
		return err
	}
	s.file = nil
	backup := s.path + ".1"
	f, err := privatefs.Open(backup, os.O_RDONLY)
	if err == nil {
		if err := f.Close(); err != nil {
			return err
		}
		if err := os.Remove(backup); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(s.path, backup); err != nil {
		return err
	}
	s.file, err = privatefs.Open(s.path, os.O_CREATE|os.O_EXCL|os.O_RDWR|os.O_APPEND)
	return err
}

// Finish releases the debug lock and reports sticky write or cleanup failures.
// Call it before emitting result data; a nil session is a disabled logger.
func (s *Session) Finish() error {
	if s == nil {
		return nil
	}
	if s.finished {
		return s.err
	}
	s.finished = true
	if s.file != nil {
		s.err = errors.Join(s.err, s.file.Sync(), s.file.Close())
	}
	if s.lockFile != nil {
		s.err = errors.Join(s.err, unlock(s.lockFile), s.lockFile.Close())
	}
	if s.err != nil {
		s.err = logFailure("debug logging failed", s.err)
	}
	return s.err
}
