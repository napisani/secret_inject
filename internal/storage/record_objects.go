package storage

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
)

const objectChunksPrefix = "object-chunks/"

// Checksums detect corrupt objects; the native store supplies protection.
type objectManifest struct {
	SchemaVersion int    `json:"schema_version"`
	Kind          string `json:"kind"`
	Generation    string `json:"generation"`
	ChunkSize     int    `json:"chunk_size"`
	Chunks        int    `json:"chunks"`
	ByteLength    int    `json:"byte_length"`
	SHA256        string `json:"sha256"`
}

func (s *RecordStore) readObject(ctx context.Context, key string) ([]byte, bool, error) {
	data, found, err := s.native.Get(ctx, key)
	if err != nil {
		return nil, false, safeStoreFailure(err)
	}
	if !found {
		return nil, false, nil
	}
	var manifest objectManifest
	if err := decodeRecord(data, &manifest, "schema_version", "kind", "generation", "chunk_size", "chunks", "byte_length", "sha256"); err != nil {
		return nil, false, err
	}
	digest, err := hex.DecodeString(manifest.SHA256)
	if err != nil || len(digest) != sha256.Size || manifest.SchemaVersion != 1 || manifest.Kind != "native_object" ||
		!validObjectGeneration(manifest.Generation) || manifest.ChunkSize < 1 || manifest.ByteLength < 0 ||
		manifest.Chunks != objectChunkCount(manifest.ByteLength, manifest.ChunkSize) {
		return nil, false, ErrRecordCorrupt
	}
	prefix := objectChunkPrefix(key) + manifest.Generation + "/"
	var value []byte
	for index := range manifest.Chunks {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		chunk, found, err := s.native.Get(ctx, prefix+strconv.Itoa(index))
		if err != nil {
			return nil, false, safeStoreFailure(err)
		}
		expected := min(manifest.ChunkSize, manifest.ByteLength-index*manifest.ChunkSize)
		if !found || len(chunk) != expected {
			return nil, false, ErrRecordCorrupt
		}
		value = append(value, chunk...)
	}
	checksum := sha256.Sum256(value)
	if !bytes.Equal(checksum[:], digest) {
		return nil, false, ErrRecordCorrupt
	}
	return value, true, nil
}

func (s *RecordStore) writeObject(ctx context.Context, key string, data []byte) error {
	generation, err := randomRecordID()
	if err != nil {
		return err
	}
	limit := s.native.MaxValueBytes()
	if limit < 1 {
		return ErrStoreUnavailable
	}
	digest := sha256.Sum256(data)
	manifest := objectManifest{
		SchemaVersion: 1,
		Kind:          "native_object",
		Generation:    generation,
		ChunkSize:     limit,
		Chunks:        objectChunkCount(len(data), limit),
		ByteLength:    len(data),
		SHA256:        hex.EncodeToString(digest[:]),
	}
	encoded, err := json.Marshal(manifest)
	if err != nil || len(encoded) > limit {
		return ErrStoreUnavailable
	}
	prefix := objectChunkPrefix(key) + generation + "/"
	for index := range manifest.Chunks {
		start := index * limit
		end := start + min(limit, len(data)-start)
		if err := s.native.Put(ctx, prefix+strconv.Itoa(index), data[start:end]); err != nil {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			return errors.Join(safeStoreFailure(err), s.removeObjectGeneration(cleanupCtx, prefix, index+1))
		}
	}
	if err := s.native.Put(ctx, key, encoded); err != nil {
		// A reported error may follow a committed manifest; its chunks must survive.
		return safeStoreFailure(err)
	}
	return s.reclaimObjectChunks(ctx, key, generation)
}

func (s *RecordStore) deleteObject(ctx context.Context, key string) error {
	if err := s.native.Delete(ctx, key); err != nil {
		return safeStoreFailure(err)
	}
	return s.reclaimObjectChunks(ctx, key, "")
}

func (s *RecordStore) removeObjectGeneration(ctx context.Context, prefix string, chunks int) error {
	for index := range chunks {
		if err := s.native.Delete(ctx, prefix+strconv.Itoa(index)); err != nil {
			return safeStoreFailure(err)
		}
	}
	return nil
}

func (s *RecordStore) reclaimObjectChunks(ctx context.Context, key, keepGeneration string) error {
	keys, err := s.native.Keys(ctx)
	if err != nil {
		return safeStoreFailure(err)
	}
	prefix := objectChunkPrefix(key)
	for _, chunkKey := range keys {
		if !strings.HasPrefix(chunkKey, prefix) {
			continue
		}
		generation, index, ok := strings.Cut(strings.TrimPrefix(chunkKey, prefix), "/")
		if !ok || !validObjectGeneration(generation) || !validChunkIndex(index) || generation == keepGeneration {
			continue
		}
		if err := s.native.Delete(ctx, chunkKey); err != nil {
			return safeStoreFailure(err)
		}
	}
	return nil
}

func (s *RecordStore) removeRoleChunks(ctx context.Context, role string) error {
	keys, err := s.native.Keys(ctx)
	if err != nil {
		return safeStoreFailure(err)
	}
	for _, key := range keys {
		parts := strings.Split(key, "/")
		if len(parts) != 5 || parts[0] != "object-chunks" || parts[1] != role ||
			!scopePattern.MatchString(parts[2]) || !validObjectGeneration(parts[3]) || !validChunkIndex(parts[4]) {
			continue
		}
		if err := s.native.Delete(ctx, key); err != nil {
			return safeStoreFailure(err)
		}
	}
	return nil
}

func objectChunkPrefix(key string) string {
	role, _, _ := strings.Cut(key, "/")
	digest := sha256.Sum256([]byte(key))
	return objectChunksPrefix + role + "/" + hex.EncodeToString(digest[:]) + "/"
}

func objectChunkCount(length, limit int) int {
	if length == 0 {
		return 1
	}
	return 1 + (length-1)/limit
}

func validChunkIndex(value string) bool {
	index, err := strconv.Atoi(value)
	return err == nil && index >= 0 && strconv.Itoa(index) == value
}

func validObjectGeneration(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 16 && value == strings.ToLower(value)
}

func randomRecordID() (string, error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", ErrStoreOperation
	}
	return hex.EncodeToString(nonce[:]), nil
}
