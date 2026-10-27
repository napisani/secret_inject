//go:build darwin && !cgo

package storage

import "context"

func openNativeAdapter(context.Context, StorePolicy) (NativeByteAdapter, error) {
	return nil, ErrStoreUnavailable
}
