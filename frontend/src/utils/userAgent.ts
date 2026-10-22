// Human-readable browser/platform names for the raw User-Agent captured on a
// session (issue #1793). The sessions table used to show the full UA string
// ("Mozilla/5.0 ... HeadlessChrome/153 ..."), which is not a device name.
//
// Deliberately small: a handful of browser/platform tokens, no UA database.
// Returns "" when nothing recognisable is present, so the caller can fall back
// to its own "unknown device" wording while still offering the raw string.

export interface UserAgentDescription {
  browser: string;
  platform: string;
  label: string;
}

function majorVersionAfter(userAgent: string, token: string): string {
  const index = userAgent.indexOf(token);
  if (index < 0) return '';
  const rest = userAgent.slice(index + token.length);
  const match = /^\d+/.exec(rest);
  return match ? match[0] : '';
}

export function describeUserAgent(userAgent: string): UserAgentDescription {
  const browserToken = (() => {
    if (userAgent.includes('Edg/')) return { name: 'Edge', token: 'Edg/' };
    if (userAgent.includes('OPR/')) return { name: 'Opera', token: 'OPR/' };
    if (userAgent.includes('SamsungBrowser/')) {
      return { name: 'Samsung Internet', token: 'SamsungBrowser/' };
    }
    if (userAgent.includes('Firefox/')) return { name: 'Firefox', token: 'Firefox/' };
    if (userAgent.includes('Chrome/')) return { name: 'Chrome', token: 'Chrome/' };
    if (userAgent.includes('Safari/')) return { name: 'Safari', token: 'Version/' };
    return null;
  })();

  let browser = '';
  if (browserToken) {
    const version = majorVersionAfter(userAgent, browserToken.token);
    browser = version ? `${browserToken.name} ${version}` : browserToken.name;
  }

  let platform = '';
  if (userAgent.includes('Windows')) platform = 'Windows';
  else if (userAgent.includes('iPhone')) platform = 'iPhone';
  else if (userAgent.includes('iPad')) platform = 'iPad';
  else if (userAgent.includes('Android')) platform = 'Android';
  else if (userAgent.includes('CrOS')) platform = 'Chrome OS';
  else if (userAgent.includes('Mac OS X') || userAgent.includes('Macintosh')) platform = 'macOS';
  else if (userAgent.includes('Linux')) platform = 'Linux';

  const label = browser && platform ? `${browser} on ${platform}` : browser || platform || '';

  return { browser, platform, label };
}

/** Short label like "Chrome 153 on Linux", or "" when unrecognisable. */
export function formatUserAgent(userAgent: string): string {
  return describeUserAgent(userAgent).label;
}
