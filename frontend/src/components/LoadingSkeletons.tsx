import { Box, Card, Paper, Skeleton, Stack } from '@mui/material';

/**
 * Skeleton loader for contact cards in the contacts list
 */
export const ContactCardSkeleton = () => {
  return (
    <Card sx={{ display: 'flex', alignItems: 'center', p: 1 }}>
      <Skeleton animation="wave" variant="circular" width={56} height={56} sx={{ mr: 2 }} />
      <Box sx={{ flex: 1 }}>
        <Skeleton animation="wave" variant="text" width="60%" height={28} />
        <Skeleton animation="wave" variant="text" width="40%" height={20} />
      </Box>
    </Card>
  );
};

/**
 * Renders multiple contact card skeletons
 */
export const ContactListSkeleton = ({ count = 5 }: { count?: number }) => {
  return (
    <Stack spacing={2}>
      {Array.from({ length: count }).map((_, index) => (
        // biome-ignore lint/suspicious/noArrayIndexKey: static placeholders, no data identity
        <ContactCardSkeleton key={index} />
      ))}
    </Stack>
  );
};

/**
 * Skeleton loader for the contact detail header
 */
export const ContactDetailHeaderSkeleton = () => {
  return (
    <Card>
      <Box sx={{ display: 'flex', p: 3, alignItems: 'flex-start' }}>
        <Skeleton animation="wave" variant="circular" width={120} height={120} sx={{ mr: 3 }} />
        <Box sx={{ flex: 1 }}>
          <Skeleton animation="wave" variant="text" width="50%" height={40} sx={{ mb: 1 }} />
          <Skeleton animation="wave" variant="text" width="30%" height={24} sx={{ mb: 2 }} />
          <Stack direction="row" spacing={1} sx={{ mb: 2 }}>
            <Skeleton animation="wave" variant="rounded" width={80} height={32} />
            <Skeleton animation="wave" variant="rounded" width={80} height={32} />
          </Stack>
          <Skeleton animation="wave" variant="text" width="70%" height={20} />
          <Skeleton animation="wave" variant="text" width="60%" height={20} />
        </Box>
      </Box>
    </Card>
  );
};

/**
 * Skeleton loader for timeline items in contact detail
 */
export const TimelineItemSkeleton = () => {
  return (
    <Box sx={{ display: 'flex', mb: 3 }}>
      <Box sx={{ mr: 2, display: 'flex', flexDirection: 'column', alignItems: 'center' }}>
        <Skeleton animation="wave" variant="circular" width={24} height={24} />
        <Skeleton animation="wave" variant="rectangular" width={2} height={60} sx={{ mt: 1 }} />
      </Box>
      <Box sx={{ flex: 1 }}>
        <Skeleton animation="wave" variant="text" width="20%" height={20} sx={{ mb: 1 }} />
        <Paper sx={{ p: 2 }}>
          <Skeleton animation="wave" variant="text" width="40%" height={24} sx={{ mb: 1 }} />
          <Skeleton animation="wave" variant="text" width="100%" height={20} />
          <Skeleton animation="wave" variant="text" width="90%" height={20} />
        </Paper>
      </Box>
    </Box>
  );
};

/**
 * Renders multiple timeline item skeletons
 */
export const TimelineSkeleton = ({ count = 3 }: { count?: number }) => {
  return (
    <Box>
      {Array.from({ length: count }).map((_, index) => (
        // biome-ignore lint/suspicious/noArrayIndexKey: static placeholders, no data identity
        <TimelineItemSkeleton key={index} />
      ))}
    </Box>
  );
};

/**
 * Skeleton loader for list items (activities, notes)
 */
export const ListItemSkeleton = () => {
  return (
    <Card sx={{ p: 2, mb: 2 }}>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 1 }}>
        <Skeleton animation="wave" variant="text" width="40%" height={24} />
        <Skeleton animation="wave" variant="text" width="20%" height={20} />
      </Box>
      <Skeleton animation="wave" variant="text" width="100%" height={20} />
      <Skeleton animation="wave" variant="text" width="80%" height={20} />
      <Box sx={{ mt: 2, display: 'flex', gap: 1 }}>
        <Skeleton animation="wave" variant="rounded" width={60} height={24} />
        <Skeleton animation="wave" variant="rounded" width={60} height={24} />
      </Box>
    </Card>
  );
};

/**
 * Renders multiple list item skeletons
 */
export const ListSkeleton = ({ count = 5 }: { count?: number }) => {
  return (
    <Box>
      {Array.from({ length: count }).map((_, index) => (
        // biome-ignore lint/suspicious/noArrayIndexKey: static placeholders, no data identity
        <ListItemSkeleton key={index} />
      ))}
    </Box>
  );
};
