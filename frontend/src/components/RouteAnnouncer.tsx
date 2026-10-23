import { useEffect, useRef } from 'react';
import { useLocation } from 'react-router';

/**
 * Hands a client-side route change to assistive technology (issue #1917).
 *
 * A normal page load moves focus and lets the screen reader discover the new
 * document. A pushState navigation does neither: after activating a nav link,
 * focus stays on the link and the user keeps hearing the old context even
 * though `document.title` changed. Move focus to the new route's `<h1>` (the
 * page's own title), falling back to the `#main-content` landmark while the
 * page is still loading. A late heading -- ContactDetailPage renders its name
 * only once the record arrives -- is picked up and focused if the user has not
 * already moved on.
 *
 * Focus is deliberately not moved on the initial mount: the browser's own page
 * load already handles that, and stealing focus on first paint would be
 * hostile. This also leaves the shared AnnouncerContext free for page status
 * messages (result counts, selections), which must not be clobbered by a
 * route announcement.
 */
export default function RouteAnnouncer() {
  const { pathname } = useLocation();
  const firstRender = useRef(true);

  useEffect(() => {
    if (firstRender.current) {
      firstRender.current = false;
      return;
    }

    const main = document.getElementById('main-content');
    if (!main) return;

    const focusHeading = (): boolean => {
      const heading = main.querySelector('h1');
      if (!heading) return false;
      heading.tabIndex = -1;
      heading.focus({ preventScroll: true });
      return true;
    };

    // Eager routes already have their heading. For a suspended route (or a
    // detail page whose name is still loading) start at main and watch for it.
    if (!focusHeading()) {
      main.focus({ preventScroll: true });
    }

    const observer = new MutationObserver(() => {
      // Only take over the heading if focus is still where we parked it; a
      // user who has already tabbed into the page keeps their place.
      if (document.activeElement !== main && document.activeElement !== document.body) {
        observer.disconnect();
        return;
      }
      if (focusHeading()) observer.disconnect();
    });
    observer.observe(main, { childList: true, subtree: true });
    const stopWatching = window.setTimeout(() => observer.disconnect(), 1500);

    return () => {
      observer.disconnect();
      window.clearTimeout(stopWatching);
    };
  }, [pathname]);

  return null;
}
