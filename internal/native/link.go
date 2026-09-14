package native

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -mmacosx-version-min=14.0
#cgo LDFLAGS: -mmacosx-version-min=14.0 -framework Foundation -framework AppKit -framework Carbon -framework CoreGraphics -framework ApplicationServices -framework Cocoa -framework QuartzCore -F/System/Library/PrivateFrameworks -framework SkyLight
*/
import "C"
