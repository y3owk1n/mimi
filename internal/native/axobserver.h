#pragma once
#include <stdbool.h>

bool AXInstallObserver(int pid);

void AXRemoveObserver(int pid);

void AXRemoveAllObservers(void);
