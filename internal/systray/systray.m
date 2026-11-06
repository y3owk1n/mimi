//
//  systray.m
//  Mimi
//
//  Copyright © 2025 Mimi. All rights reserved.
//

#import "systray.h"

#import "../native/mimi_log.h"

#import <Cocoa/Cocoa.h>
#include <CoreGraphics/CoreGraphics.h>
#include <dlfcn.h>

#pragma mark - External Function Declarations

extern void systray_menu_item_selected(int menuId);
extern void systray_on_ready(void);
extern void systray_on_exit(void);

#pragma mark - Workspace Title Observer

static id gWorkspaceObserver = nil;
static id gActiveDisplayObserver = nil;
static NSAttributedString *gLastWorkspaceTitle = nil;

// The keys of one entry spacesInFrontFromSkyLight returns.
static NSString *const kTitleNumberKey = @"number";
static NSString *const kTitleFocusedKey = @"focused";
static NSString *const kTitleXKey = @"x";
static NSString *const kTitleYKey = @"y";

/// The space in front on every display, ordered the way mimi counts displays,
/// left to right and then top to bottom. Each entry holds the space's 1-based
/// Mission Control number, the number mimi action space takes, and whether its
/// display has keyboard focus. nil when SkyLight cannot be read.
static NSArray<NSDictionary *> *spacesInFrontFromSkyLight(void) {
	static void *skyLight = NULL;
	static dispatch_once_t onceToken;
	dispatch_once(&onceToken, ^{
		skyLight = dlopen("/System/Library/PrivateFrameworks/SkyLight.framework/SkyLight", RTLD_LAZY);
		if (!skyLight)
			MimiLog(MimiLogLevelWarn, @"SkyLight not loaded, the menu bar shows no space number", nil);
	});

	if (!skyLight)
		return nil;

	// Function pointer types for private SkyLight APIs.
	typedef int (*SLSMainConnectionIDFunc)(void);
	typedef CFStringRef (*SLSCopyActiveMenuBarDisplayIdentifierFunc)(int);
	typedef CFArrayRef (*SLSCopyManagedDisplaySpacesFunc)(int);

	SLSMainConnectionIDFunc SLSMainConnectionID = (SLSMainConnectionIDFunc)dlsym(skyLight, "SLSMainConnectionID");
	if (!SLSMainConnectionID) {
		SLSMainConnectionID = (SLSMainConnectionIDFunc)dlsym(skyLight, "CGSMainConnectionID");
	}

	SLSCopyActiveMenuBarDisplayIdentifierFunc SLSCopyActiveMenuBarDisplayIdentifier =
	    (SLSCopyActiveMenuBarDisplayIdentifierFunc)dlsym(skyLight, "SLSCopyActiveMenuBarDisplayIdentifier");
	if (!SLSCopyActiveMenuBarDisplayIdentifier) {
		SLSCopyActiveMenuBarDisplayIdentifier =
		    (SLSCopyActiveMenuBarDisplayIdentifierFunc)dlsym(skyLight, "CGSCopyActiveMenuBarDisplayIdentifier");
	}

	SLSCopyManagedDisplaySpacesFunc SLSCopyManagedDisplaySpaces =
	    (SLSCopyManagedDisplaySpacesFunc)dlsym(skyLight, "SLSCopyManagedDisplaySpaces");

	if (!SLSMainConnectionID || !SLSCopyActiveMenuBarDisplayIdentifier || !SLSCopyManagedDisplaySpaces) {
		static dispatch_once_t missingOnce;
		dispatch_once(&missingOnce, ^{
			MimiLog(
			    MimiLogLevelWarn, @"SkyLight symbols missing, the menu bar shows no space number", @{
				    @"main_connection" : @(SLSMainConnectionID != NULL),
				    @"active_menu_bar_display" : @(SLSCopyActiveMenuBarDisplayIdentifier != NULL),
				    @"managed_display_spaces" : @(SLSCopyManagedDisplaySpaces != NULL),
			    });
		});
		return nil;
	}

	int conn = SLSMainConnectionID();

	// The display with the menu bar is the one with keyboard focus. The value
	// is a UUID string matching the "Display Identifier" key below.
	CFStringRef activeDisplayUUID = SLSCopyActiveMenuBarDisplayIdentifier(conn);
	if (!activeDisplayUUID) {
		return nil;
	}

	CFArrayRef managed = SLSCopyManagedDisplaySpaces(conn);
	if (!managed) {
		CFRelease(activeDisplayUUID);
		return nil;
	}

	NSMutableArray<NSDictionary *> *found = [NSMutableArray array];
	int counter = 0;

	for (NSDictionary *display in (__bridge NSArray *)managed) {
		NSString *displayID = display[@"Display Identifier"];
		BOOL isActive = displayID != nil &&
		                CFStringCompare((__bridge CFStringRef)displayID, activeDisplayUUID, 0) == kCFCompareEqualTo;

		NSDictionary *currentSpace = display[@"Current Space"];
		NSNumber *currentSpaceID = currentSpace[@"ManagedSpaceID"] ?: currentSpace[@"id64"];

		NSArray *spaces = display[@"Spaces"];
		if (![spaces isKindOfClass:[NSArray class]]) {
			continue;
		}

		int number = -1;

		for (id spaceDict in spaces) {
			counter++;

			if (![spaceDict isKindOfClass:[NSDictionary class]]) {
				continue;
			}

			id sid = spaceDict[@"id64"];
			if (![sid isKindOfClass:[NSNumber class]]) {
				sid = spaceDict[@"ManagedSpaceID"];
			}

			if ([sid isKindOfClass:[NSNumber class]] && currentSpaceID != nil &&
			    [sid unsignedLongLongValue] == [currentSpaceID unsignedLongLongValue]) {
				number = counter;
			}
		}

		if (number < 0) {
			continue;
		}

		// With displays sharing one set of spaces the identifier names no
		// display, and the bounds stay empty, which is fine for one entry.
		CGRect bounds = CGRectZero;
		CFUUIDRef uuid = displayID ? CFUUIDCreateFromString(NULL, (__bridge CFStringRef)displayID) : NULL;
		if (uuid) {
			bounds = CGDisplayBounds(CGDisplayGetDisplayIDFromUUID(uuid));
			CFRelease(uuid);
		}

		[found addObject:@{
			kTitleNumberKey : @(number),
			kTitleFocusedKey : @(isActive),
			kTitleXKey : @(bounds.origin.x),
			kTitleYKey : @(bounds.origin.y),
		}];
	}

	CFRelease(managed);
	CFRelease(activeDisplayUUID);

	[found sortUsingComparator:^NSComparisonResult(NSDictionary *a, NSDictionary *b) {
		NSComparisonResult byX = [a[kTitleXKey] compare:b[kTitleXKey]];

		return byX != NSOrderedSame ? byX : [a[kTitleYKey] compare:b[kTitleYKey]];
	}];

	return found;
}

#pragma mark - Static State

static BOOL _showSystray = YES;
static BOOL _exitCalled = NO;

#pragma mark - App Delegate

@interface AppDelegate : NSObject <NSApplicationDelegate, NSMenuDelegate>
@property(strong) NSStatusItem *statusItem;
@property(strong) NSMenu *menu;
@end

@implementation AppDelegate

- (void)applicationDidFinishLaunching:(NSNotification *)notification {
	if (_showSystray) {
		self.statusItem = [[NSStatusBar systemStatusBar] statusItemWithLength:NSVariableStatusItemLength];
		self.menu = [[NSMenu alloc] init];
		[self.menu setAutoenablesItems:NO];
		[self.menu setDelegate:self];
		[self.statusItem setMenu:self.menu];
	}

	// Notify Go that we are ready
	systray_on_ready();
}

- (void)applicationWillTerminate:(NSNotification *)notification {
	if (!_exitCalled) {
		_exitCalled = YES;
		systray_on_exit();
	}
}

- (void)itemClicked:(id)sender {
	NSMenuItem *item = (NSMenuItem *)sender;
	systray_menu_item_selected((int)[item tag]);
}

@end

#pragma mark - Native Loop Functions

AppDelegate *appDelegate;

void internalNativeLoop(void) {
	@autoreleasepool {
		[NSApplication sharedApplication];
		appDelegate = [[AppDelegate alloc] init];
		[NSApp setDelegate:appDelegate];
		[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
		[NSApp run];
	}
}

void MimiNativeLoop(void) {
	_showSystray = YES;
	internalNativeLoop();
}

void MimiNativeLoopHeadless(void) {
	_showSystray = NO;
	internalNativeLoop();
}

void MimiQuit(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		if (!_exitCalled) {
			_exitCalled = YES;
			systray_on_exit();
		}

		[NSApp stop:nil];

		// [NSApp stop:] sets a flag checked on the *next* event loop
		// iteration, so post a dummy event to wake the loop immediately.
		NSEvent *event = [NSEvent otherEventWithType:NSEventTypeApplicationDefined
		                                    location:NSZeroPoint
		                               modifierFlags:0
		                                   timestamp:0
		                                windowNumber:0
		                                     context:nil
		                                     subtype:0
		                                       data1:0
		                                       data2:0];
		[NSApp postEvent:event atStart:YES];
	});
}

#pragma mark - Status Item Functions

void MimiSetIcon(const char *iconBytes, int length, bool isTemplate) {
	// Copy the icon bytes before dispatching so the caller can free
	// the original buffer immediately after this function returns.
	NSData *data = [NSData dataWithBytes:iconBytes length:length];

	dispatch_async(dispatch_get_main_queue(), ^{
		NSImage *image = [[NSImage alloc] initWithData:data];

		// Menu bar icons are 22×22 points (44×44 @2x retina). Setting the size
		// explicitly ensures macOS renders the icon at the correct dimensions
		// regardless of the source PNG pixel size.
		[image setSize:NSMakeSize(22, 22)];
		[image setTemplate:isTemplate];

		if (appDelegate && appDelegate.statusItem) {
			appDelegate.statusItem.button.title = @"";
			[appDelegate.statusItem.button setImagePosition:NSImageOnly];
			appDelegate.statusItem.button.image = image;
		}
	});
}

void MimiSetTitle(const char *title) {
	NSString *str = [NSString stringWithUTF8String:title];

	dispatch_async(dispatch_get_main_queue(), ^{
		if (appDelegate && appDelegate.statusItem) {
			appDelegate.statusItem.button.image = nil;
			[appDelegate.statusItem.button setImagePosition:NSNoImage];
			appDelegate.statusItem.button.title = str;
		}
	});
}

void MimiSetTooltip(const char *tooltip) {
	NSString *str = [NSString stringWithUTF8String:tooltip];

	dispatch_async(dispatch_get_main_queue(), ^{
		if (appDelegate && appDelegate.statusItem) {
			appDelegate.statusItem.button.toolTip = str;
		}
	});
}

/// The space number read from the on-screen window list, for when SkyLight
/// gives none, or -1.
static int activeWorkspaceNumberFromWindowList(void) {
	@autoreleasepool {
		MimiLog(MimiLogLevelDebug, @"space number not found through SkyLight, reading the window list", nil);

		CFArrayRef windowList = CGWindowListCopyWindowInfo(
		    kCGWindowListOptionOnScreenOnly | kCGWindowListExcludeDesktopElements, kCGNullWindowID);
		if (windowList) {
			int workspace = -1;
			for (NSDictionary *info in (__bridge NSArray *)windowList) {
				NSNumber *layer = info[(__bridge NSString *)kCGWindowLayer];
				int l = layer ? [layer intValue] : 0;
				if (l != 0) {
					continue;
				}

				id ws = info[@"kCGWindowWorkspace"];
				if ([ws isKindOfClass:[NSNumber class]]) {
					workspace = [ws intValue];
					break;
				}

				ws = info[@"Workspace"];
				if ([ws isKindOfClass:[NSNumber class]]) {
					workspace = [ws intValue];
					break;
				}
			}

			CFRelease(windowList);
			if (workspace >= 0) {
				return workspace;
			}
		}

		return -1;
	}
}

#pragma mark - Menu Item Lookup

NSMenuItem *findItemByTagInMenu(NSMenu *menu, int menuId) {
	if (!menu)
		return nil;

	// Check top-level items first
	NSMenuItem *item = [menu itemWithTag:menuId];
	if (item)
		return item;

	// Recursively search submenus
	for (NSMenuItem *menuItem in [menu itemArray]) {
		if ([menuItem hasSubmenu]) {
			item = findItemByTagInMenu([menuItem submenu], menuId);
			if (item)
				return item;
		}
	}

	return nil;
}

NSMenuItem *findItemByTag(int menuId) {
	if (!appDelegate || !appDelegate.menu)
		return nil;

	return findItemByTagInMenu(appDelegate.menu, menuId);
}

#pragma mark - Helper Functions

void runOnMainThread(void (^block)(void)) {
	if ([NSThread isMainThread]) {
		block();
	} else {
		dispatch_async(dispatch_get_main_queue(), block);
	}
}

#pragma mark - Workspace Title Observer

/// The menu bar title for the spaces in front: one display's number as plain
/// text, or every display's numbers joined by " | " with the focused display's
/// in bold. All menu bars show the same title, which is why it carries every
/// display. With no space from SkyLight it is the window list's number, or M.
static NSAttributedString *workspaceTitle(NSArray<NSDictionary *> *spaces) {
	NSFont *font = [NSFont menuBarFontOfSize:0];

	if (spaces.count < 2) {
		NSString *text = @"M";

		if (spaces.count == 1) {
			text = [spaces[0][kTitleNumberKey] stringValue];
		} else {
			int workspace = activeWorkspaceNumberFromWindowList();
			if (workspace >= 0) {
				text = [NSString stringWithFormat:@"%d", workspace + 1];
			}
		}

		return [[NSAttributedString alloc] initWithString:text attributes:@{NSFontAttributeName : font}];
	}

	NSFont *bold = [[NSFontManager sharedFontManager] convertFont:font toHaveTrait:NSBoldFontMask];
	NSMutableAttributedString *title = [[NSMutableAttributedString alloc] init];

	for (NSUInteger index = 0; index < spaces.count; index++) {
		if (index > 0) {
			[title appendAttributedString:[[NSAttributedString alloc] initWithString:@" | "
			                                                              attributes:@{NSFontAttributeName : font}]];
		}

		NSDictionary *space = spaces[index];
		NSFont *numberFont = [space[kTitleFocusedKey] boolValue] ? bold : font;
		[title appendAttributedString:[[NSAttributedString alloc] initWithString:[space[kTitleNumberKey] stringValue]
		                                                              attributes:@{NSFontAttributeName : numberFont}]];
	}

	return title;
}

static void refreshWorkspaceTitle(void) {
	if (!appDelegate || !appDelegate.statusItem) {
		return;
	}

	NSAttributedString *title = workspaceTitle(spacesInFrontFromSkyLight());
	if ([title isEqualToAttributedString:gLastWorkspaceTitle]) {
		return;
	}

	gLastWorkspaceTitle = title;

	appDelegate.statusItem.button.image = nil;
	[appDelegate.statusItem.button setImagePosition:NSNoImage];
	appDelegate.statusItem.button.attributedTitle = title;
}

static void scheduleWorkspaceTitleRefresh(void) {
	refreshWorkspaceTitle();
	dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(0.03 * NSEC_PER_SEC)), dispatch_get_main_queue(), ^{
		refreshWorkspaceTitle();
	});
}

void MimiStartWorkspaceTitleObserver(void) {
	runOnMainThread(^{
		if (gWorkspaceObserver) {
			scheduleWorkspaceTitleRefresh();
			return;
		}

		scheduleWorkspaceTitleRefresh();

		NSNotificationCenter *nc = [[NSWorkspace sharedWorkspace] notificationCenter];

		gWorkspaceObserver = [nc addObserverForName:NSWorkspaceActiveSpaceDidChangeNotification
		                                     object:nil
		                                      queue:[NSOperationQueue mainQueue]
		                                 usingBlock:^(__unused NSNotification *note) {
			                                 scheduleWorkspaceTitleRefresh();
		                                 }];

		gActiveDisplayObserver = [nc addObserverForName:@"NSWorkspaceActiveDisplayDidChangeNotification"
		                                         object:nil
		                                          queue:[NSOperationQueue mainQueue]
		                                     usingBlock:^(__unused NSNotification *note) {
			                                     scheduleWorkspaceTitleRefresh();
		                                     }];
	});
}

void MimiStopWorkspaceTitleObserver(void) {
	runOnMainThread(^{
		NSNotificationCenter *nc = [[NSWorkspace sharedWorkspace] notificationCenter];

		if (gWorkspaceObserver) {
			[nc removeObserver:gWorkspaceObserver];
			gWorkspaceObserver = nil;
		}

		if (gActiveDisplayObserver) {
			[nc removeObserver:gActiveDisplayObserver];
			gActiveDisplayObserver = nil;
		}

		gLastWorkspaceTitle = nil;
	});
}

void MimiRefreshWorkspaceTitle(void) {
	runOnMainThread(^{
		scheduleWorkspaceTitleRefresh();
	});
}

#pragma mark - Menu Item Functions

void MimiAddMenuItem(int menuId, const char *title, short disabled, short checked) {
	NSString *titleStr = [NSString stringWithUTF8String:title];

	runOnMainThread(^{
		NSMenuItem *item = [[NSMenuItem alloc] initWithTitle:titleStr action:@selector(itemClicked:) keyEquivalent:@""];
		[item setTarget:appDelegate];
		[item setTag:menuId];
		[item setEnabled:!disabled];
		[item setState:checked ? NSControlStateValueOn : NSControlStateValueOff];

		[appDelegate.menu addItem:item];
	});
}

void MimiAddSubMenuItem(int parentId, int menuId, const char *title, short disabled, short checked) {
	NSString *titleStr = [NSString stringWithUTF8String:title];

	runOnMainThread(^{
		NSMenuItem *parent = findItemByTag(parentId);
		if (!parent)
			return;

		if (![parent submenu]) {
			NSMenu *submenu = [[NSMenu alloc] init];
			[submenu setAutoenablesItems:NO];
			[parent setSubmenu:submenu];
		}

		NSMenuItem *item = [[NSMenuItem alloc] initWithTitle:titleStr action:@selector(itemClicked:) keyEquivalent:@""];
		[item setTarget:appDelegate];
		[item setTag:menuId];
		[item setEnabled:!disabled];
		[item setState:checked ? NSControlStateValueOn : NSControlStateValueOff];

		[[parent submenu] addItem:item];
	});
}

void MimiAddSeparator(int parentId) {
	runOnMainThread(^{
		if (parentId == 0) {
			[appDelegate.menu addItem:[NSMenuItem separatorItem]];
			return;
		}

		NSMenuItem *parent = findItemByTag(parentId);
		if (!parent)
			return;

		if (![parent submenu]) {
			NSMenu *submenu = [[NSMenu alloc] init];
			[submenu setAutoenablesItems:NO];
			[parent setSubmenu:submenu];
		}

		[[parent submenu] addItem:[NSMenuItem separatorItem]];
	});
}

void MimiHideMenuItem(int menuId) {
	runOnMainThread(^{
		NSMenuItem *item = findItemByTag(menuId);
		if (item)
			[item setHidden:YES];
	});
}

void MimiShowMenuItem(int menuId) {
	runOnMainThread(^{
		NSMenuItem *item = findItemByTag(menuId);
		if (item)
			[item setHidden:NO];
	});
}

void MimiSetItemChecked(int menuId, short checked) {
	runOnMainThread(^{
		NSMenuItem *item = findItemByTag(menuId);
		if (item)
			[item setState:checked ? NSControlStateValueOn : NSControlStateValueOff];
	});
}

void MimiSetItemDisabled(int menuId, short disabled) {
	runOnMainThread(^{
		NSMenuItem *item = findItemByTag(menuId);
		if (item)
			[item setEnabled:!disabled];
	});
}

void MimiSetItemTitle(int menuId, const char *title) {
	NSString *str = [NSString stringWithUTF8String:title];

	runOnMainThread(^{
		NSMenuItem *item = findItemByTag(menuId);
		if (item)
			[item setTitle:str];
	});
}
