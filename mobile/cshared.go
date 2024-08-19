//go:build mobile
// +build mobile

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

//export start
func start(extraArgs *C.char, callback C.CCallback) {
	Start(C.GoString(extraArgs), WrapCallbackCgo(callback))
}

//export getStatus
func getStatus() int32 {
	return lndStarted
}

func main() {}
