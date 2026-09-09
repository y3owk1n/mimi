#pragma once

int MimiCheckAccessibilityPermissions(void);
int MimiRequestAccessibilityPermissions(void);
int MimiShowAccessibilityPermissionStartupAlert(void);
int MimiShowConfigOnboardingAlert(const char *configPath);
/// Report whether macOS lets mimi capture the screen. Never prompts.
int MimiCheckScreenCapturePermission(void);
/// Clear the standing Screen Recording decision and ask macOS again, which
/// prompts. Reports the decision as it stands after the request.
int MimiRequestScreenCapturePermission(void);
/// Show the startup guidance for granting Screen Recording. Returns 1 when
/// granted, 2 when the user chose to skip animation, 3 when mimi must restart
/// for a grant to take effect.
int MimiShowScreenCapturePermissionStartupAlert(void);
