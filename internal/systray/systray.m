//
//  systray.m
//  Mimi
//
//  Copyright © 2025 Mimi. All rights reserved.
//

#import "systray.h"

#import <Cocoa/Cocoa.h>
#include <CoreGraphics/CoreGraphics.h>
#include <dlfcn.h>
#include <limits.h>

#pragma mark - External Function Declarations

extern void systray_menu_item_selected(int menuId);
extern void systray_on_ready(void);
extern void systray_on_exit(void);

#pragma mark - Workspace Title Observer

static id gWorkspaceObserver = nil;
static int gLastWorkspaceTitle = INT_MIN;

static int activeWorkspaceNumberFromSkyLight(void) {
	static void *skyLight = NULL;
	static dispatch_once_t onceToken;
	dispatch_once(&onceToken, ^{
		skyLight = dlopen("/System/Library/PrivateFrameworks/SkyLight.framework/SkyLight", RTLD_LAZY);
	});

	if (!skyLight) {
		return -1;
	}

	typedef int (*SLSMainConnectionIDFunc)(void);
	typedef uint64_t (*SLSGetActiveSpaceFunc)(int);
	typedef CFArrayRef (*SLSCopyManagedDisplaySpacesFunc)(int);

	SLSMainConnectionIDFunc SLSMainConnectionID = (SLSMainConnectionIDFunc)dlsym(skyLight, "SLSMainConnectionID");
	if (!SLSMainConnectionID) {
		SLSMainConnectionID = (SLSMainConnectionIDFunc)dlsym(skyLight, "CGSMainConnectionID");
	}

	SLSGetActiveSpaceFunc SLSGetActiveSpace = (SLSGetActiveSpaceFunc)dlsym(skyLight, "SLSGetActiveSpace");
	if (!SLSGetActiveSpace) {
		SLSGetActiveSpace = (SLSGetActiveSpaceFunc)dlsym(skyLight, "CGSGetActiveSpace");
	}

	SLSCopyManagedDisplaySpacesFunc SLSCopyManagedDisplaySpaces =
	    (SLSCopyManagedDisplaySpacesFunc)dlsym(skyLight, "SLSCopyManagedDisplaySpaces");

	if (!SLSMainConnectionID || !SLSGetActiveSpace || !SLSCopyManagedDisplaySpaces) {
		return -1;
	}

	int conn = SLSMainConnectionID();
	uint64_t active = SLSGetActiveSpace(conn);
	if (active == 0) {
		return -1;
	}

	CFArrayRef managed = SLSCopyManagedDisplaySpaces(conn);
	if (!managed) {
		return -1;
	}

	int counter = 0;
	NSArray *displays = (__bridge NSArray *)managed;
	for (NSDictionary *display in displays) {
		NSArray *spaces = display[@"Spaces"];
		if (![spaces isKindOfClass:[NSArray class]]) {
			continue;
		}

		for (id spaceDict in spaces) {
			if (![spaceDict isKindOfClass:[NSDictionary class]]) {
				continue;
			}

			id sid = spaceDict[@"id64"];
			if (![sid isKindOfClass:[NSNumber class]]) {
				sid = spaceDict[@"ManagedSpaceID"];
			}
			if (![sid isKindOfClass:[NSNumber class]]) {
				counter++;
				continue;
			}

			if ((uint64_t)[sid unsignedLongLongValue] == active) {
				CFRelease(managed);
				return counter;
			}

			counter++;
		}
	}

	CFRelease(managed);
	return -1;
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

void MimiRegisterSystray(void) {
	// Placeholder if needed for init
}

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

int MimiGetActiveWorkspaceNumber(void) {
	@autoreleasepool {
		int skyLightIndex = activeWorkspaceNumberFromSkyLight();
		if (skyLightIndex >= 0) {
			return skyLightIndex;
		}

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

static void applyWorkspaceTitle(int workspaceNumber) {
	if (!appDelegate || !appDelegate.statusItem) {
		return;
	}

	NSString *title = workspaceNumber >= 0 ? [NSString stringWithFormat:@"%d", workspaceNumber + 1] : @"M";

	appDelegate.statusItem.button.image = nil;
	[appDelegate.statusItem.button setImagePosition:NSNoImage];
	appDelegate.statusItem.button.title = title;
}

static void refreshWorkspaceTitle(void) {
	int workspace = MimiGetActiveWorkspaceNumber();
	if (workspace == gLastWorkspaceTitle) {
		return;
	}

	gLastWorkspaceTitle = workspace;
	applyWorkspaceTitle(workspace);
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

		gWorkspaceObserver = [[[NSWorkspace sharedWorkspace] notificationCenter]
		    addObserverForName:NSWorkspaceActiveSpaceDidChangeNotification
		                object:nil
		                 queue:[NSOperationQueue mainQueue]
		            usingBlock:^(__unused NSNotification *note) {
			            scheduleWorkspaceTitleRefresh();
		            }];
	});
}

void MimiStopWorkspaceTitleObserver(void) {
	runOnMainThread(^{
		if (gWorkspaceObserver) {
			[[[NSWorkspace sharedWorkspace] notificationCenter] removeObserver:gWorkspaceObserver];
			gWorkspaceObserver = nil;
		}

		gLastWorkspaceTitle = INT_MIN;
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
