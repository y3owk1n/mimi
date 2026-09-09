#pragma once

int MimiCheckAccessibilityPermissions(void);
int MimiRequestAccessibilityPermissions(void);
int MimiShowAccessibilityPermissionStartupAlert(void);
int MimiShowConfigOnboardingAlert(const char *configPath);
/// Ask macOS for Screen Recording permission, which prompts the first time
/// and otherwise reports the standing decision.
int MimiRequestScreenCapturePermission(void);
