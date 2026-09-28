package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

static void setDockIcon(const void *data, int length) {
	NSData *d = [NSData dataWithBytes:data length:length];
	dispatch_async(dispatch_get_main_queue(), ^{
		NSImage *img = [[NSImage alloc] initWithData:d];
		if (img != nil) {
			[[NSApplication sharedApplication] setApplicationIconImage:img];
		}
	});
}
*/
import "C"

import "unsafe"

// setAppIcon replaces the generic executable icon in the Dock. It's needed
// when running the bare binary (e.g. `make run`) rather than an .app bundle.
func setAppIcon() {
	C.setDockIcon(unsafe.Pointer(&iconPNG[0]), C.int(len(iconPNG)))
}
