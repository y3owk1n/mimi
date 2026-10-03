#import "mimi_log.h"

extern int mimiNativeLogEnabled(int level);
extern void mimiNativeLog(int level, char *message, char *fields);

void MimiLog(MimiLogLevel level, NSString *_Nonnull message, NSDictionary<NSString *, id> *_Nullable fields) {
	// Skip building the fields for a line the daemon would drop.
	if (!mimiNativeLogEnabled((int)level))
		return;

	@autoreleasepool {
		NSString *json = nil;
		if (fields.count > 0) {
			NSData *data = [NSJSONSerialization dataWithJSONObject:fields options:0 error:nil];
			if (data)
				json = [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding];
		}

		mimiNativeLog((int)level, (char *)[message UTF8String], json ? (char *)[json UTF8String] : NULL);
	}
}
