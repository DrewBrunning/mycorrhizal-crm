import { useTranslation } from 'react-i18next';
import NotFoundState from './components/NotFoundState';

export default function NotFoundPage() {
  const { t } = useTranslation();
  return <NotFoundState title={t('notFound.title')} description={t('notFound.description')} />;
}
