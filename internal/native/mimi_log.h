#ifndef MIMI_LOG_H
#define MIMI_LOG_H

#import <Foundation/Foundation.h>

// Matches the levels internal/native/nativelog.go maps onto the daemon's
// logger.
typedef NS_ENUM(int, MimiLogLevel) {
	MimiLogLevelDebug = 0,
	MimiLogLevelInfo = 1,
	MimiLogLevelWarn = 2,
};

// MimiLog logs through the daemon's logger, so native lines carry the same
// levels and reach the same sinks as the Go side. message is fixed text with
// no values in it. Each entry of fields becomes its own log field, and its
// values must be strings or numbers. Never pass a window title or other user
// payload. MimiLog is defined in internal/native, so a package outside it
// that calls MimiLog must import native to link.
void MimiLog(MimiLogLevel level, NSString *_Nonnull message, NSDictionary<NSString *, id> *_Nullable fields);

#endif  // MIMI_LOG_H
