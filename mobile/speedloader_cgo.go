package main

/*
#ifndef CALLBACK_DEFS_H
#define CALLBACK_DEFS_H

#include <stdlib.h>

typedef void (*ResponseFunc)(void* context, const char* data, int length);
typedef void (*ErrorFunc)(void* context, const char* error);

typedef struct CCallback {
    ResponseFunc onResponse;
    ErrorFunc onError;
    void* responseContext;
    void* errorContext;
} CCallback;

#endif // CALLBACK_DEFS_H
*/
import "C"

//export gossipSync
func gossipSync(serviceUrl *C.char, cacheDir *C.char, dataDir *C.char, networkType *C.char, callback C.CCallback) {
	goServiceUrl := C.GoString(serviceUrl)
	goCacheDir := C.GoString(cacheDir)
	goDataDir := C.GoString(dataDir)
	goNetworkType := C.GoString(networkType)

	go func() {
		GossipSync(goServiceUrl, goCacheDir, goDataDir, goNetworkType, WrapCallbackCgo(callback))
	}()
}

//export cancelGossipSync
func cancelGossipSync() {
	CancelGossipSync()
}
