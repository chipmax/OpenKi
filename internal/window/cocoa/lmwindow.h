// lmwindow.h is the whole C surface of the macOS window layer.
//
// Two shapes of call, both safe from any thread:
//   - lm_run builds the NSWindow + WKWebView. Main thread only, called once
//     from systray's ready hook inside NSApp.run.
//   - everything else is a lm_do_* operation that hops to the main queue
//     itself, because that is the only thread AppKit allows touching windows.
#ifndef ZGWINDOW_H
#define ZGWINDOW_H

extern void lm_run(const char *title, int width, int height, const char *url);

// Builds the window on the main queue. getlantern/systray hands its ready
// callback to a fresh goroutine, so the caller cannot rely on being on the main
// thread already; this bounces into NSApp.run's queue and calls back into Go's
// lmGoCreate().
extern void lm_dispatch_create(void);

// Frees a string allocated by Go's C.CString.
extern void lm_free(void *p);

// Stands in for the Windows-only SetAppUserModelID so cmd/openki can stay
// platform-free.
extern void lm_noop_string(const char *unused);

// Window operations.
extern void lm_do_minimize(void);
extern void lm_do_toggle_max(void);
extern void lm_do_show(void);
extern void lm_do_hide(void);
extern void lm_do_terminate(void);
extern int lm_do_is_visible(void);
extern int lm_do_is_minimized(void);
extern int lm_do_is_maximized(void);

// Bounds use the platform-neutral top-left-origin convention documented on
// window.Bounds; the flip to AppKit's bottom-left screens happens in here.
extern void lm_do_set_bounds(int x, int y, int w, int h);
extern void lm_do_get_bounds(void);
extern void lm_do_get_screen_frame(void);

// Static C scratch buffers the two getters above fill. They are read straight
// after the (blocking) getter returns, so no Go pointer ever crosses into C.
extern int *lm_bounds(void);
extern int *lm_screen(void);

#endif