package storage

import (
	"context"
	"errors"
	"time"

	"github.com/godbus/dbus"
)

const secretService = "org.freedesktop.secrets"
const secretPath dbus.ObjectPath = "/org/freedesktop/secrets"
const secretAPI = "org.freedesktop.Secret."

type linuxNative struct {
	conn       *dbus.Conn
	collection dbus.ObjectPath
	session    dbus.ObjectPath
	policy     StorePolicy
}

type dbusSecret struct {
	Session     dbus.ObjectPath
	Parameters  []byte
	Value       []byte
	ContentType string
}

func secretFailure(err error) error {
	if err == nil {
		return nil
	}
	var failure dbus.Error
	if errors.As(err, &failure) && failure.Name == secretAPI+"Error.IsLocked" {
		return ErrInteractionRequired
	}
	return safeStoreFailure(err)
}

func (l *linuxNative) call(ctx context.Context, path dbus.ObjectPath, method string, args ...interface{}) *dbus.Call {
	return l.conn.Object(secretService, path).CallWithContext(ctx, method, 0, args...)
}
func (l *linuxNative) property(ctx context.Context, path dbus.ObjectPath, iface, name string, target interface{}) error {
	var value dbus.Variant
	if err := l.call(ctx, path, "org.freedesktop.DBus.Properties.Get", iface, name).Store(&value); err != nil {
		return secretFailure(err)
	}
	if err := dbus.Store([]interface{}{value.Value()}, target); err != nil {
		return ErrRecordCorrupt
	}
	return nil
}

func openNativeAdapter(ctx context.Context, policy StorePolicy) (NativeByteAdapter, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	conn, err := dbus.SessionBusPrivate()
	if err != nil {
		return nil, ErrStoreUnavailable
	}
	l := &linuxNative{conn: conn, policy: policy}
	fail := func(err error) (NativeByteAdapter, error) { return nil, errors.Join(err, l.Close()) }
	if err := conn.Auth(nil); err != nil {
		return fail(ErrStoreUnavailable)
	}
	if err := conn.Hello(); err != nil {
		return fail(ErrStoreUnavailable)
	}
	var output dbus.Variant
	if err := l.call(ctx, secretPath, secretAPI+"Service.OpenSession", "plain", dbus.MakeVariant("")).Store(&output, &l.session); err != nil {
		return fail(secretFailure(err))
	}
	if l.session == "/" || !l.session.IsValid() {
		return fail(ErrStoreUnavailable)
	}
	var collections []dbus.ObjectPath
	if err := l.property(ctx, secretPath, secretAPI+"Service", "Collections", &collections); err != nil {
		return fail(err)
	}
	for _, path := range collections {
		var label string
		if err := l.property(ctx, path, secretAPI+"Collection", "Label", &label); err != nil {
			return fail(err)
		}
		legacyPath := string(secretPath) + "/collection/secret_inject"
		encodedPath := string(secretPath) + "/collection/secret_5finject"
		if label == "secret_inject" || string(path) == legacyPath || string(path) == encodedPath {
			if l.collection != "" {
				return fail(ErrStoreUnavailable)
			}
			l.collection = path
		}
	}
	if l.collection == "" {
		if policy != InteractiveSetup {
			return fail(ErrInteractionRequired)
		}
		properties := map[string]dbus.Variant{secretAPI + "Collection.Label": dbus.MakeVariant("secret_inject")}
		var prompt dbus.ObjectPath
		if err := l.call(ctx, secretPath, secretAPI+"Service.CreateCollection", properties, "").Store(&l.collection, &prompt); err != nil {
			return fail(secretFailure(err))
		}
		if prompt != "/" {
			result, err := l.prompt(ctx, prompt)
			if err != nil {
				return fail(err)
			}
			if err := dbus.Store([]interface{}{result.Value()}, &l.collection); err != nil {
				return fail(ErrStoreOperation)
			}
		}
		if l.collection == "/" || !l.collection.IsValid() {
			return fail(ErrStoreUnavailable)
		}
	}
	if err := l.unlocked(ctx, l.collection, "Collection"); err != nil {
		return fail(err)
	}
	return l, nil
}

func (l *linuxNative) prompt(ctx context.Context, path dbus.ObjectPath) (result dbus.Variant, err error) {
	if l.policy != InteractiveSetup || path == "/" || !path.IsValid() {
		return dbus.Variant{}, ErrInteractionRequired
	}
	options := []dbus.MatchOption{dbus.WithMatchObjectPath(path), dbus.WithMatchInterface(secretAPI + "Prompt"), dbus.WithMatchMember("Completed")}
	if err := l.conn.AddMatchSignal(options...); err != nil {
		return dbus.Variant{}, secretFailure(err)
	}
	defer func() { err = errors.Join(err, secretFailure(l.conn.RemoveMatchSignal(options...)), ctx.Err()) }()
	signals := make(chan *dbus.Signal, 8)
	l.conn.Signal(signals)
	defer l.conn.RemoveSignal(signals)
	if err := l.call(ctx, path, secretAPI+"Prompt.Prompt", "").Err; err != nil {
		return dbus.Variant{}, secretFailure(err)
	}
	for {
		select {
		case <-ctx.Done():
			return dbus.Variant{}, ctx.Err()
		case signal, ok := <-signals:
			if !ok {
				return dbus.Variant{}, ErrStoreUnavailable
			}
			if signal.Path != path || signal.Name != secretAPI+"Prompt.Completed" {
				continue
			}
			var dismissed bool
			var result dbus.Variant
			if err := dbus.Store(signal.Body, &dismissed, &result); err != nil {
				return dbus.Variant{}, ErrRecordCorrupt
			}
			if dismissed {
				return dbus.Variant{}, ErrInteractionRequired
			}
			return result, nil
		}
	}
}

func (l *linuxNative) unlocked(ctx context.Context, path dbus.ObjectPath, kind string) error {
	var locked bool
	if err := l.property(ctx, path, secretAPI+kind, "Locked", &locked); err != nil {
		return err
	}
	if !locked {
		return nil
	}
	if l.policy != InteractiveSetup {
		return ErrInteractionRequired
	}
	var unlocked []dbus.ObjectPath
	var prompt dbus.ObjectPath
	if err := l.call(ctx, secretPath, secretAPI+"Service.Unlock", []dbus.ObjectPath{path}).Store(&unlocked, &prompt); err != nil {
		return secretFailure(err)
	}
	if prompt != "/" {
		if _, err := l.prompt(ctx, prompt); err != nil {
			return err
		}
	}
	if err := l.property(ctx, path, secretAPI+kind, "Locked", &locked); err != nil {
		return err
	}
	if locked {
		return ErrInteractionRequired
	}
	return nil
}
func (l *linuxNative) item(ctx context.Context, key string) (dbus.ObjectPath, bool, error) {
	if err := l.unlocked(ctx, l.collection, "Collection"); err != nil {
		return "", false, err
	}
	var paths []dbus.ObjectPath
	if err := l.call(ctx, l.collection, secretAPI+"Collection.SearchItems", map[string]string{"profile": key}).Store(&paths); err != nil {
		return "", false, secretFailure(err)
	}
	if len(paths) == 0 {
		return "", false, nil
	}
	if len(paths) != 1 {
		return "", false, ErrStoreUnavailable
	}
	if err := l.unlocked(ctx, paths[0], "Item"); err != nil {
		return "", false, err
	}
	return paths[0], true, nil
}
func (*linuxNative) MaxValueBytes() int { return 2048 }
func (l *linuxNative) Get(ctx context.Context, key string) ([]byte, bool, error) {
	path, found, err := l.item(ctx, key)
	if err != nil || !found {
		return nil, false, err
	}
	var secret dbusSecret
	if err := l.call(ctx, path, secretAPI+"Item.GetSecret", l.session).Store(&secret); err != nil {
		return nil, false, secretFailure(err)
	}
	if secret.Session != l.session || len(secret.Parameters) != 0 {
		return nil, false, ErrRecordCorrupt
	}
	return secret.Value, true, nil
}
func (l *linuxNative) Put(ctx context.Context, key string, value []byte) error {
	if len(value) > l.MaxValueBytes() {
		return ErrStoreOperation
	}
	path, found, err := l.item(ctx, key)
	if err != nil {
		return err
	}
	secret := dbusSecret{Session: l.session, Parameters: []byte{}, Value: value, ContentType: "application/octet-stream"}
	if found {
		return secretFailure(l.call(ctx, path, secretAPI+"Item.SetSecret", secret).Err)
	}
	properties := map[string]dbus.Variant{secretAPI + "Item.Label": dbus.MakeVariant(key), secretAPI + "Item.Attributes": dbus.MakeVariant(map[string]string{"profile": key})}
	var prompt dbus.ObjectPath
	if err := l.call(ctx, l.collection, secretAPI+"Collection.CreateItem", properties, secret, true).Store(&path, &prompt); err != nil {
		return secretFailure(err)
	}
	if prompt != "/" {
		_, err := l.prompt(ctx, prompt)
		return err
	}
	if path == "/" || !path.IsValid() {
		return ErrStoreOperation
	}
	return nil
}
func (l *linuxNative) Delete(ctx context.Context, key string) error {
	path, found, err := l.item(ctx, key)
	if err != nil || !found {
		return err
	}
	var prompt dbus.ObjectPath
	if err := l.call(ctx, path, secretAPI+"Item.Delete").Store(&prompt); err != nil {
		return secretFailure(err)
	}
	if prompt != "/" {
		_, err := l.prompt(ctx, prompt)
		return err
	}
	return nil
}
func (l *linuxNative) Keys(ctx context.Context) ([]string, error) {
	if err := l.unlocked(ctx, l.collection, "Collection"); err != nil {
		return nil, err
	}
	var paths []dbus.ObjectPath
	if err := l.property(ctx, l.collection, secretAPI+"Collection", "Items", &paths); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(paths))
	seen := make(map[string]bool)
	for _, path := range paths {
		if err := l.unlocked(ctx, path, "Item"); err != nil {
			return nil, err
		}
		var attrs map[string]string
		if err := l.property(ctx, path, secretAPI+"Item", "Attributes", &attrs); err != nil {
			return nil, err
		}
		if key, ok := attrs["profile"]; ok {
			if seen[key] {
				return nil, ErrStoreUnavailable
			}
			seen[key] = true
			keys = append(keys, key)
		}
	}
	return keys, nil
}
func (l *linuxNative) Close() error {
	var err error
	if l.session != "" && l.session != "/" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err = secretFailure(l.call(ctx, l.session, secretAPI+"Session.Close").Err)
		cancel()
		l.session = ""
	}
	if l.conn != nil {
		err = errors.Join(err, safeStoreFailure(l.conn.Close()))
		l.conn = nil
	}
	return err
}
