// Copy-to-clipboard helper (T34). navigator.clipboard.writeText requires a
// secure context (HTTPS or localhost) and user-gesture; falls back to the
// classic hidden-textarea + document.execCommand('copy') trick for
// non-secure contexts / older browsers, matching the copy affordance's
// universal-fallback intent.
export async function copyToClipboard(text: string): Promise<boolean> {
  if (navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      // Fall through to the legacy fallback below.
    }
  }

  try {
    const textarea = document.createElement('textarea');
    textarea.value = text;
    // Keep it off-screen and non-interactive so it never flashes visibly.
    textarea.style.position = 'fixed';
    textarea.style.top = '-1000px';
    textarea.style.left = '-1000px';
    textarea.style.opacity = '0';
    document.body.appendChild(textarea);
    textarea.focus();
    textarea.select();
    const ok = document.execCommand('copy');
    document.body.removeChild(textarea);
    return ok;
  } catch {
    return false;
  }
}
