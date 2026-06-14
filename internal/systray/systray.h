#ifndef SYSTRAY_H
#define SYSTRAY_H

#include <stdbool.h>

void MimiNativeLoop(void);
void MimiNativeLoopHeadless(void);
void MimiQuit(void);

void MimiSetIcon(const char *iconBytes, int length, bool isTemplate);
void MimiSetTitle(const char *title);
void MimiSetTooltip(const char *tooltip);
int MimiGetActiveWorkspaceNumber(void);
void MimiStartWorkspaceTitleObserver(void);
void MimiStopWorkspaceTitleObserver(void);
void MimiRefreshWorkspaceTitle(void);

void MimiAddMenuItem(int menuId, const char *title, short disabled, short checked);
void MimiAddSubMenuItem(int parentId, int menuId, const char *title, short disabled, short checked);
void MimiAddSeparator(int parentId);
void MimiHideMenuItem(int menuId);
void MimiShowMenuItem(int menuId);
void MimiSetItemChecked(int menuId, short checked);
void MimiSetItemDisabled(int menuId, short disabled);
void MimiSetItemTitle(int menuId, const char *title);

#endif
