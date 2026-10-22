import { Box, Button, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';
import { useDocumentTitle } from '../hooks/useDocumentTitle';

interface NotFoundStateProps {
  title: string;
  description: string;
}

// Issue #1793: a missing route or record used to be a silent redirect or a
// bare sentence with no way back. Both not-found surfaces share this state so
// the page keeps its own title and always offers a route into the app.
export default function NotFoundState({ title, description }: NotFoundStateProps) {
  const { t } = useTranslation();
  useDocumentTitle(title);

  return (
    <Box sx={{ maxWidth: 600, mx: 'auto', mt: 4, p: 2, textAlign: 'center' }}>
      <Typography variant="h5" component="h1" gutterBottom>
        {title}
      </Typography>
      <Typography variant="body1" color="text.secondary" sx={{ mb: 3 }}>
        {description}
      </Typography>
      <Box sx={{ display: 'flex', gap: 1, justifyContent: 'center', flexWrap: 'wrap' }}>
        <Button component={Link} to="/contacts" variant="contained">
          {t('notFound.goToContacts')}
        </Button>
        <Button component={Link} to="/" variant="outlined">
          {t('notFound.goToDashboard')}
        </Button>
      </Box>
    </Box>
  );
}
