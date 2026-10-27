//go:build darwin && cgo

package storage

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
// Legacy file keychains require APIs covered by -Wdeprecated-declarations.
#cgo CFLAGS: -Wno-deprecated-declarations
#include <Security/Security.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdlib.h>

static OSStatus si_open(const char *path, int setup, SecKeychainRef *keychain) {
    OSStatus status = SecKeychainOpen(path, keychain);
    SecKeychainStatus state = 0;
    // SecKeychainOpen may return a reference before checking file existence.
    if (status == errSecSuccess)
        status = SecKeychainGetStatus(*keychain, &state);
    if (status == errSecNoSuchKeychain && setup) {
        if (*keychain) CFRelease(*keychain);
        *keychain = NULL;
        status = SecKeychainCreate(path, 0, NULL, true, NULL, keychain);
        if (status == errSecSuccess)
            status = SecKeychainGetStatus(*keychain, &state);
    }
    if (status != errSecSuccess) return status;
    if (!(state & kSecUnlockStateStatus)) {
        if (!setup) return errSecInteractionNotAllowed;
        // A null password requires usePassword=false to request OS authentication.
        return SecKeychainUnlock(*keychain, 0, NULL, false);
    }
    return errSecSuccess;
}
static OSStatus si_ready(SecKeychainRef keychain) {
    SecKeychainStatus state;
    OSStatus status = SecKeychainGetStatus(keychain, &state);
    if (status != errSecSuccess) return status;
    return state & kSecUnlockStateStatus ? errSecSuccess : errSecInteractionNotAllowed;
}
static OSStatus si_get(SecKeychainRef keychain, const char *key, UInt32 keylen, UInt32 *size, void **data) {
    OSStatus status = si_ready(keychain);
    if (status != errSecSuccess) return status;
    return SecKeychainFindGenericPassword(keychain, 13, "secret_inject", keylen, key, size, data, NULL);
}
static OSStatus si_put(SecKeychainRef keychain, const char *key, UInt32 keylen, const void *data, UInt32 size) {
    OSStatus status = si_ready(keychain);
    if (status != errSecSuccess) return status;
    SecKeychainItemRef item = NULL;
    status = SecKeychainFindGenericPassword(keychain, 13, "secret_inject", keylen, key, NULL, NULL, &item);
    if (status == errSecItemNotFound)
        return SecKeychainAddGenericPassword(keychain, 13, "secret_inject", keylen, key, size, data, NULL);
    if (status != errSecSuccess) return status;
    status = SecKeychainItemModifyAttributesAndData(item, NULL, size, data);
    CFRelease(item);
    return status;
}
static OSStatus si_delete(SecKeychainRef keychain, const char *key, UInt32 keylen) {
    OSStatus status = si_ready(keychain);
    if (status != errSecSuccess) return status;
    SecKeychainItemRef item = NULL;
    status = SecKeychainFindGenericPassword(keychain, 13, "secret_inject", keylen, key, NULL, NULL, &item);
    if (status == errSecItemNotFound) return errSecSuccess;
    if (status != errSecSuccess) return status;
    status = SecKeychainItemDelete(item);
    CFRelease(item);
    return status;
}
static OSStatus si_keys(SecKeychainRef keychain, CFArrayRef *result) {
    OSStatus status = si_ready(keychain);
    if (status != errSecSuccess) return status;
    CFMutableDictionaryRef query = CFDictionaryCreateMutable(NULL, 0, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
    CFArrayRef list = CFArrayCreate(NULL, (const void **)&keychain, 1, &kCFTypeArrayCallBacks);
    CFDictionarySetValue(query, kSecClass, kSecClassGenericPassword);
    CFDictionarySetValue(query, kSecAttrService, CFSTR("secret_inject"));
    CFDictionarySetValue(query, kSecMatchSearchList, list);
    CFDictionarySetValue(query, kSecMatchLimit, kSecMatchLimitAll);
    CFDictionarySetValue(query, kSecReturnAttributes, kCFBooleanTrue);
    CFTypeRef value = NULL;
    status = SecItemCopyMatching(query, &value);
    CFRelease(list);
    CFRelease(query);
    if (status == errSecItemNotFound) {
        *result = CFArrayCreate(NULL, NULL, 0, &kCFTypeArrayCallBacks);
        return errSecSuccess;
    }
    if (status != errSecSuccess) return status;
    if (!value || CFGetTypeID(value) != CFArrayGetTypeID()) {
        if (value) CFRelease(value);
        return errSecDecode;
    }
    *result = (CFArrayRef)value;
    return errSecSuccess;
}
static char *si_key(CFArrayRef array, CFIndex index) {
    CFTypeRef entry = CFArrayGetValueAtIndex(array, index);
    if (!entry || CFGetTypeID(entry) != CFDictionaryGetTypeID()) return NULL;
    CFTypeRef account = CFDictionaryGetValue((CFDictionaryRef)entry, kSecAttrAccount);
    if (!account || CFGetTypeID(account) != CFStringGetTypeID()) return NULL;
    CFIndex size = CFStringGetMaximumSizeForEncoding(CFStringGetLength((CFStringRef)account), kCFStringEncodingUTF8) + 1;
    char *value = malloc(size);
    if (!value) return NULL;
    if (!CFStringGetCString((CFStringRef)account, value, size, kCFStringEncodingUTF8)) { free(value); return NULL; }
    return value;
}
static void si_release_array(CFArrayRef ref) { if (ref) CFRelease(ref); }
static void si_release_keychain(SecKeychainRef ref) { if (ref) CFRelease(ref); }
*/
import "C"

import (
	"context"
	"errors"
	"os/user"
	"path/filepath"
	"sync"
	"unsafe"
)

var keychainOperations sync.Mutex

type macNative struct {
	keychain C.SecKeychainRef
	policy   StorePolicy
}

type keychainFailure struct{ Status int32 }

func (e *keychainFailure) Error() string { return "native keychain operation failed" }
func (e *keychainFailure) Is(target error) bool {
	if e.Status == int32(C.errSecInteractionNotAllowed) || e.Status == int32(C.errSecAuthFailed) || e.Status == int32(C.errSecUserCanceled) {
		return target == ErrInteractionRequired
	}
	if e.Status == int32(C.errSecNoSuchKeychain) || e.Status == int32(C.errSecNotAvailable) {
		return target == ErrStoreUnavailable
	}
	return target == ErrStoreOperation
}

func keychainStatus(status C.OSStatus) error {
	if status == C.errSecSuccess {
		return nil
	}
	return &keychainFailure{Status: int32(status)}
}

func (m *macNative) operation(ctx context.Context, run func() C.OSStatus) (err error) {
	keychainOperations.Lock()
	defer keychainOperations.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	var previous C.Boolean
	if status := C.SecKeychainGetUserInteractionAllowed(&previous); status != C.errSecSuccess {
		return keychainStatus(status)
	}
	allowed := C.Boolean(0)
	if m.policy == InteractiveSetup {
		allowed = 1
	}
	if status := C.SecKeychainSetUserInteractionAllowed(allowed); status != C.errSecSuccess {
		return keychainStatus(status)
	}
	defer func() {
		err = errors.Join(err, keychainStatus(C.SecKeychainSetUserInteractionAllowed(previous)), ctx.Err())
	}()
	return keychainStatus(run())
}

func openNativeAdapter(ctx context.Context, policy StorePolicy) (NativeByteAdapter, error) {
	m := &macNative{policy: policy}
	setup := C.int(0)
	if policy == InteractiveSetup {
		setup = 1
	}
	account, err := user.Current()
	if err != nil {
		return nil, ErrStoreUnavailable
	}
	path := C.CString(filepath.Join(account.HomeDir, "Library", "Keychains", "secret_inject.keychain"))
	defer C.free(unsafe.Pointer(path))
	err = m.operation(ctx, func() C.OSStatus { return C.si_open(path, setup, &m.keychain) })
	if err != nil {
		return nil, errors.Join(err, m.Close())
	}
	return m, nil
}

func (m *macNative) MaxValueBytes() int { return 2048 }
func (m *macNative) Get(ctx context.Context, key string) ([]byte, bool, error) {
	name := C.CString(key)
	defer C.free(unsafe.Pointer(name))
	var size C.UInt32
	var data unsafe.Pointer
	var status C.OSStatus
	var value []byte
	err := m.operation(ctx, func() C.OSStatus {
		status = C.si_get(m.keychain, name, C.UInt32(len(key)), &size, &data)
		if status == C.errSecItemNotFound {
			return C.errSecSuccess
		}
		if status != C.errSecSuccess {
			return status
		}
		if uint64(size) > uint64(1<<31-1) {
			freeStatus := C.SecKeychainItemFreeContent(nil, data)
			if freeStatus != C.errSecSuccess {
				return freeStatus
			}
			return C.errSecDecode
		}
		value = C.GoBytes(data, C.int(size))
		return C.SecKeychainItemFreeContent(nil, data)
	})
	if err != nil {
		return nil, false, err
	}
	if status == C.errSecItemNotFound {
		return nil, false, nil
	}
	return value, true, nil
}
func (m *macNative) Put(ctx context.Context, key string, value []byte) error {
	if len(value) > m.MaxValueBytes() {
		return ErrStoreOperation
	}
	name := C.CString(key)
	defer C.free(unsafe.Pointer(name))
	data := C.CBytes(value)
	defer C.free(data)
	return m.operation(ctx, func() C.OSStatus { return C.si_put(m.keychain, name, C.UInt32(len(key)), data, C.UInt32(len(value))) })
}
func (m *macNative) Delete(ctx context.Context, key string) error {
	name := C.CString(key)
	defer C.free(unsafe.Pointer(name))
	return m.operation(ctx, func() C.OSStatus { return C.si_delete(m.keychain, name, C.UInt32(len(key))) })
}
func (m *macNative) Keys(ctx context.Context) ([]string, error) {
	var values C.CFArrayRef
	err := m.operation(ctx, func() C.OSStatus { return C.si_keys(m.keychain, &values) })
	if values != 0 {
		defer C.si_release_array(values)
	}
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, int(C.CFArrayGetCount(values)))
	for index := C.CFIndex(0); index < C.CFArrayGetCount(values); index++ {
		value := C.si_key(values, index)
		if value == nil {
			return nil, ErrRecordCorrupt
		}
		keys = append(keys, C.GoString(value))
		C.free(unsafe.Pointer(value))
	}
	return keys, nil
}
func (m *macNative) Close() error {
	keychainOperations.Lock()
	defer keychainOperations.Unlock()
	if m.keychain != 0 {
		C.si_release_keychain(m.keychain)
		m.keychain = 0
	}
	return nil
}
