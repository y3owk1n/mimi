#ifndef MIMI_LOG_H
#define MIMI_LOG_H

#import <Foundation/Foundation.h>

// MIMI_LOG prepends a "Mimi: " prefix to all log lines for greppability in
// Console.app and `log show`. Centralized so the prefix can't drift across
// files.
#define MIMI_LOG(fmt, ...) NSLog(@"Mimi: " fmt, ##__VA_ARGS__)

#endif  // MIMI_LOG_H
