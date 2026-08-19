import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';

// #211: every route calls this with its own visible title so `document.title`
// and the page's `h1` can't drift apart -- see the model comment block at the
// top of App.tsx.
export function useDocumentTitle(title?: string) {
  const { t } = useTranslation();
  useEffect(() => {
    const appName = t('app.title');
    document.title = title ? `${title} · ${appName}` : appName;
  }, [title, t]);
}
