import AddIcon from '@mui/icons-material/Add';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import {
  Alert,
  Box,
  Button,
  Chip,
  FormControlLabel,
  IconButton,
  Link as MuiLink,
  Stack,
  Switch,
  Tooltip,
  Typography,
} from '@mui/material';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';
import type { ContactEntityLink } from '../api/entities';
import { useSnackbar } from '../context/SnackbarContext';
import type { useContactEntityLinks } from '../hooks/useContactEntityLinks';
import { ENTITY_SENSITIVITY_KEYS, roleKey } from '../utils/entityLabels';
import { formatLinkPeriod } from '../utils/entityPeriod';
import { getErrorMessage } from '../utils/errorHandler';
import { formatAddressLine } from '../utils/linkResolution';
import ContactEntityLinkDialog from './ContactEntityLinkDialog';
import LinkFromOwnValueDialog, { type OwnValue } from './LinkFromOwnValueDialog';

export type ContactEntityLinksApi = ReturnType<typeof useContactEntityLinks>;

interface OwnAddress {
  id?: string;
  components?: { kind: string; value: string }[];
}

interface ContactEntityLinksPanelProps {
  api: ContactEntityLinksApi;
  /** The contact's own (stored, non-projected) organisations and addresses. */
  ownOrganizations: { id?: string; name?: string }[];
  ownAddresses: OwnAddress[];
}

function addressLabel(a: OwnAddress): string {
  const find = (kind: string) => a.components?.find((c) => c.kind === kind)?.value ?? '';
  return formatAddressLine({
    street: find('name'),
    city: find('locality'),
    region: find('region'),
    postal: find('postcode'),
    country: find('country'),
  });
}

// The contact page's "Organisations, places & pets" section body (issue #2301,
// ADR 0043 decisions 2, 3 and 10): the contact's links with role, dates and the
// primary switch, plus add / edit / remove, adopt, and link-from-own-value.
export default function ContactEntityLinksPanel({
  api,
  ownOrganizations,
  ownAddresses,
}: ContactEntityLinksPanelProps) {
  const { t } = useTranslation();
  const { showError, showSuccess } = useSnackbar();
  const [dialog, setDialog] = useState<{ link: ContactEntityLink | null } | null>(null);
  const [fromOwnOpen, setFromOwnOpen] = useState(false);

  const ownValues = useMemo<OwnValue[]>(
    () => [
      ...ownOrganizations
        .map((o, index) => ({ o, index }))
        .filter(({ o }) => o.name)
        .map(({ o, index }) => ({
          kind: 'organization' as const,
          id: o.id,
          index,
          label: o.name as string,
        })),
      ...ownAddresses
        .map((a, index) => ({ a, index, label: addressLabel(a) }))
        .filter(({ label }) => label)
        .map(({ a, index, label }) => ({ kind: 'address' as const, id: a.id, index, label })),
    ],
    [ownOrganizations, ownAddresses],
  );

  const run = async (action: () => Promise<unknown>, success?: string) => {
    try {
      await action();
      if (success) showSuccess(success);
    } catch (err) {
      showError(getErrorMessage(err));
    }
  };

  const handleRemove = (link: ContactEntityLink) => {
    if (!window.confirm(t('contactDetail.entityLinks.removeConfirm', { name: link.entity_name })))
      return;
    void run(() => api.remove(link));
  };

  return (
    <Box>
      <Box sx={{ display: 'flex', gap: 1, flexWrap: 'wrap', mb: 1.5 }}>
        <Button
          variant="contained"
          size="small"
          startIcon={<AddIcon />}
          onClick={() => setDialog({ link: null })}
        >
          {t('contactDetail.entityLinks.add')}
        </Button>
        <Button
          variant="outlined"
          size="small"
          disabled={ownValues.length === 0}
          onClick={() => setFromOwnOpen(true)}
        >
          {t('contactDetail.entityLinks.linkFromOwn')}
        </Button>
      </Box>

      {api.error && <Alert severity="error">{api.error}</Alert>}
      {!api.error && api.links.length === 0 && !api.loading && (
        <Typography color="text.secondary">{t('contactDetail.entityLinks.empty')}</Typography>
      )}

      <Stack component="ul" spacing={1.5} sx={{ listStyle: 'none', p: 0, m: 0 }}>
        {api.links.map((link) => {
          const key = roleKey(link.kind, link.role);
          const period = formatLinkPeriod(link.period, t);
          const projects = link.kind !== 'pet';
          return (
            <Box
              component="li"
              key={link.id}
              sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 1.5 }}
            >
              <Box sx={{ display: 'flex', alignItems: 'flex-start', gap: 1, flexWrap: 'wrap' }}>
                <Box sx={{ flex: 1, minWidth: 200 }}>
                  <MuiLink
                    component={Link}
                    to={`/entities/${link.entity_id}`}
                    sx={{ fontWeight: 500 }}
                  >
                    {link.entity_name}
                  </MuiLink>
                  <Box
                    sx={{
                      display: 'flex',
                      gap: 0.75,
                      alignItems: 'center',
                      flexWrap: 'wrap',
                      mt: 0.25,
                    }}
                  >
                    <Typography variant="body2">{key ? t(key) : link.role}</Typography>
                    {link.role_label && (
                      <Typography variant="body2" color="text.secondary">
                        {link.role_label}
                      </Typography>
                    )}
                    {period && (
                      <Typography variant="body2" color="text.secondary">
                        {period}
                      </Typography>
                    )}
                  </Box>
                  <Box sx={{ display: 'flex', gap: 0.5, flexWrap: 'wrap', mt: 0.5 }}>
                    {link.projects_as === 'ORG' && (
                      <Chip
                        size="small"
                        color="primary"
                        label={t('contactDetail.entityLinks.shownAsOrg')}
                      />
                    )}
                    {link.projects_as === 'ADR' && (
                      <Chip
                        size="small"
                        color="primary"
                        label={t('contactDetail.entityLinks.shownAsAdr')}
                      />
                    )}
                    {link.is_primary && !link.current && (
                      <Chip size="small" label={t('contactDetail.entityLinks.notCurrent')} />
                    )}
                    {link.sensitivity !== 'normal' && (
                      <Chip
                        size="small"
                        color="warning"
                        label={t(ENTITY_SENSITIVITY_KEYS[link.sensitivity])}
                      />
                    )}
                  </Box>
                </Box>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, flexWrap: 'wrap' }}>
                  {projects && (
                    <FormControlLabel
                      control={
                        <Switch
                          size="small"
                          checked={link.is_primary}
                          onChange={(e) => void run(() => api.setPrimary(link, e.target.checked))}
                          slotProps={{
                            input: {
                              'aria-label': t('contactDetail.entityLinks.primaryFor', {
                                name: link.entity_name,
                              }),
                            },
                          }}
                        />
                      }
                      label={t('contactDetail.entityLinks.primary')}
                    />
                  )}
                  {projects && (
                    <Tooltip title={t('contactDetail.entityLinks.adoptHint')}>
                      <IconButton
                        size="small"
                        aria-label={
                          link.kind === 'place'
                            ? t('contactDetail.entityLinks.adoptPlace', { name: link.entity_name })
                            : t('contactDetail.entityLinks.adoptOrganisation', {
                                name: link.entity_name,
                              })
                        }
                        onClick={() =>
                          void run(
                            () => api.adopt(link),
                            t('contactDetail.entityLinks.adopted', { name: link.entity_name }),
                          )
                        }
                      >
                        <ContentCopyIcon fontSize="small" />
                      </IconButton>
                    </Tooltip>
                  )}
                  <IconButton
                    size="small"
                    aria-label={t('contactDetail.entityLinks.editFor', { name: link.entity_name })}
                    onClick={() => setDialog({ link })}
                  >
                    <EditIcon fontSize="small" />
                  </IconButton>
                  <IconButton
                    size="small"
                    color="error"
                    aria-label={t('contactDetail.entityLinks.removeFor', {
                      name: link.entity_name,
                    })}
                    onClick={() => handleRemove(link)}
                  >
                    <DeleteIcon fontSize="small" />
                  </IconButton>
                </Box>
              </Box>
            </Box>
          );
        })}
      </Stack>

      <ContactEntityLinkDialog
        open={dialog !== null}
        link={dialog?.link ?? null}
        onClose={() => setDialog(null)}
        onSave={async (input, editing) => {
          await api.save(input, editing);
          showSuccess(t('contactDetail.entityLinks.saved'));
        }}
      />
      <LinkFromOwnValueDialog
        open={fromOwnOpen}
        values={ownValues}
        onClose={() => setFromOwnOpen(false)}
        onLink={async (input) => {
          const result = await api.linkFromExisting(input);
          showSuccess(
            result.entity_created
              ? t('contactDetail.entityLinks.fromOwn.createdAndLinked', {
                  name: result.entity.name,
                })
              : t('contactDetail.entityLinks.fromOwn.linked', { name: result.entity.name }),
          );
        }}
      />
    </Box>
  );
}
