#ifndef MIMI_ANIMATE_H
#define MIMI_ANIMATE_H

#import <Foundation/Foundation.h>

/// One window a frame animation moves: the window server's number and the
/// frame it is being given, in window coordinates.
typedef struct {
	uint32_t number;
	double x;
	double y;
	double w;
	double h;
} MimiAnimationTarget;

/// The easing curves MimiAnimationBegin takes.
enum {
	MimiEasingLinear = 0,
	MimiEasingEaseIn = 1,
	MimiEasingEaseOut = 2,
	MimiEasingEaseInOut = 3,
};

/// Prepare an animation of the given windows from where they are now to the
/// given frames, over duration seconds. It captures the screen and puts a
/// still of it over the real windows, so the caller can write the real frames
/// right after without anything moving on screen. MimiAnimationStart then
/// moves pictures of the windows over the still. An animation already
/// running is replaced, and its windows start from where they are on screen.
///
/// Returns the number of windows that will animate, 0 when nothing could be
/// captured, or -1 when Screen Recording is not granted. It never prompts.
int MimiAnimationBegin(const MimiAnimationTarget *targets, int count, double duration, int easing);

/// Start the animation MimiAnimationBegin prepared, leaving out the windows
/// whose frames did not land. Without a prepared animation it does nothing.
void MimiAnimationStart(const uint32_t *dropped, int count);

/// Make the window per display the animation draws in, ahead of the first
/// animation.
void MimiAnimationWarm(void);

#endif  // MIMI_ANIMATE_H
