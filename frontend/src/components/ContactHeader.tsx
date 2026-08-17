import { Box, Card, CardContent, Avatar, Typography, Chip, IconButton, Stack, TextField, Autocomplete, Button, SvgIcon, Menu, MenuItem, ListItemText, ListItemIcon, useTheme, useMediaQuery } from '@mui/material';
import { useState } from 'react';
import MoreVertIcon from '@mui/icons-material/MoreVert';
import StarIcon from '@mui/icons-material/Star';
import StarBorderIcon from '@mui/icons-material/StarBorder';
import GroupIcon from '@mui/icons-material/Group';
import LocalOfferIcon from '@mui/icons-material/LocalOffer';
import EditIcon from '@mui/icons-material/Edit';
import SaveIcon from '@mui/icons-material/Save';
import CloseIcon from '@mui/icons-material/Close';
import DeleteIcon from '@mui/icons-material/Delete';
import CameraAltIcon from '@mui/icons-material/CameraAlt';
import AutoModeIcon from '@mui/icons-material/AutoMode';
import ArchiveIcon from '@mui/icons-material/Archive';
import UnarchiveIcon from '@mui/icons-material/Unarchive';
import MergeIcon from '@mui/icons-material/MergeType';
import ShareIcon from '@mui/icons-material/Share';
import PersonIcon from '@mui/icons-material/Person';
import { mdiDownloadOutline, mdiNoteMultipleOutline } from '@mdi/js';
import { useTranslation } from 'react-i18next';
import { ContactFieldKey, resolveEnabledFields } from '../contactFields';
import { ContactRecordResponse, nameComponentValue } from '../api/contacts';
import LanguageField from './LanguageField';
import { Circle } from '../api/circles';
import { Tag } from '../api/tags';

export interface ProfileValues {
  prefix: string;
  firstname: string;
  middle_name: string;
  lastname: string;
  suffix: string;
  nickname: string;
  // CRMEnvelope.Kind (T27): human|animal.
  kind: string;
  // Card.Kind (T29): individual|group|org|location|application|device.
  cardKind: string;
  // Card.Language (T29): default language tag.
  language: string;
}

interface ContactHeaderProps {
  record: ContactRecordResponse;
  profilePic: string;
  editingProfile: boolean;
  profileValues: ProfileValues;
  enabledFields?: Set<ContactFieldKey>;
  contactCircles: Circle[];
  contactTags: Tag[];
  allCircles: Circle[];
  allTags: Tag[];
  onStartEditProfile: () => void;
  onCancelEditProfile: () => void;
  onSaveProfile: () => void;
  onDeleteContact: () => void;
  onProfileValueChange: (values: ProfileValues) => void;
  onAddCircle: (circle: Circle) => void;
  onRemoveCircle: (circle: Circle) => void;
  onAddTag: (tag: Tag) => void;
  onRemoveTag: (tag: Tag) => void;
  onUploadProfilePicture: () => void;
  onStayInTouch?: () => void;
  onArchiveContact?: () => void;
  onUnarchiveContact?: () => void;
  // Issue #173: favorite toggle, rendered as a star beside the name.
  onToggleFavorite?: () => void;
  onMergeContact?: () => void;
  onPrepView?: () => void;
  onShareContact?: () => void;
  onExportContact: (format: string) => void;
  // T90: whether this contact is the caller's "Me" contact, and the handler to
  // set/clear that pointer from the header's overflow menu.
  isMe?: boolean;
  onToggleMe?: () => void;
}

export default function ContactHeader({
  record,
  profilePic,
  editingProfile,
  profileValues,
  enabledFields,
  contactCircles,
  contactTags,
  allCircles,
  allTags,
  onStartEditProfile,
  onCancelEditProfile,
  onSaveProfile,
  onDeleteContact,
  onProfileValueChange,
  onAddCircle,
  onRemoveCircle,
  onAddTag,
  onRemoveTag,
  onUploadProfilePicture,
  onStayInTouch,
  onArchiveContact,
  onUnarchiveContact,
  onToggleFavorite,
  onMergeContact,
  onPrepView,
  onShareContact,
  onExportContact,
  isMe,
  onToggleMe
}: ContactHeaderProps) {
  const { t } = useTranslation();
  const enabled = enabledFields ?? resolveEnabledFields(null);
  const [exportMenuAnchor, setExportMenuAnchor] = useState<HTMLElement | null>(null);
  const exportMenuOpen = Boolean(exportMenuAnchor);
  // Overflow menu for the header action buttons on narrow viewports (T28).
  const [actionsMenuAnchor, setActionsMenuAnchor] = useState<HTMLElement | null>(null);
  const actionsMenuOpen = Boolean(actionsMenuAnchor);
  // Collapse the text buttons (stay in touch / merge / export / archive) into
  // a single overflow menu below the `md` breakpoint. The buttons overflow the
  // viewport well above phone widths as the window shrinks (v0.4.1 testing),
  // so the collapse point is md, not sm. The edit pencil stays visible at
  // every size.
  const theme = useTheme();
  const compactActions = useMediaQuery(theme.breakpoints.down('md'));
  const isOn = (key: ContactFieldKey) => enabled.has(key);

  const card = record.card || {};
  const prefix = nameComponentValue(card.name?.components, 'title');
  const firstname = nameComponentValue(card.name?.components, 'given') || '';
  const middleName = nameComponentValue(card.name?.components, 'given2');
  const lastname = nameComponentValue(card.name?.components, 'surname') || '';
  const suffix = nameComponentValue(card.name?.components, 'generation');
  const nickname = card.nicknames?.[0]?.name;
  const kind = record.crm?.kind || '';
  const archived = record.archived;

  // Circle/tag editing state
  const [editingCircles, setEditingCircles] = useState(false);
  const [editingTags, setEditingTags] = useState(false);
  const [newCircleName, setNewCircleName] = useState('');
  const [newTagName, setNewTagName] = useState('');

  const displayName = [
    prefix,
    firstname,
    nickname ? `"${nickname}"` : '',
    middleName,
    lastname,
    suffix,
  ]
    .map((part) => part?.trim())
    .filter(Boolean)
    .join(' ');

  return (
    <Card sx={{ mb: 1.5 }}>
      <CardContent sx={{ py: 1.5, '&:last-child': { pb: 1.5 } }}>
        <Box sx={{ display: 'flex', alignItems: 'flex-start' }}>
          <Box
            sx={{
              position: 'relative',
              '&:hover .camera-badge': {
                opacity: 1
              }
            }}
          >
            <Avatar
              src={profilePic || undefined}
              sx={{
                width: 90,
                height: 90,
                cursor: 'pointer',
                bgcolor: 'primary.main',
                fontSize: '2rem',
                '&:hover': { opacity: 0.8 }
              }}
              onClick={onUploadProfilePicture}
            >
              {firstname.charAt(0)}
            </Avatar>
            <IconButton
              className="camera-badge"
              size="small"
              onClick={onUploadProfilePicture}
              sx={{
                position: 'absolute',
                bottom: -4,
                right: -4,
                bgcolor: 'primary.main',
                color: 'white',
                width: 22,
                height: 22,
                opacity: 0,
                transition: 'opacity 0.2s',
                '&:hover': { bgcolor: 'primary.dark' }
              }}
            >
              <CameraAltIcon sx={{ fontSize: 14 }} />
            </IconButton>
          </Box>
          <Box sx={{ flex: 1, ml: 1.5 }}>
            {editingProfile ? (
              // Edit Mode
              <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                {isOn('prefix') && (
                  <TextField
                    label={t('contacts.prefix')}
                    value={profileValues.prefix}
                    onChange={(e) => onProfileValueChange({ ...profileValues, prefix: e.target.value })}
                    size="small"
                  />
                )}
                <TextField
                  label={t('contactDetail.firstname')}
                  value={profileValues.firstname}
                  onChange={(e) => onProfileValueChange({ ...profileValues, firstname: e.target.value })}
                  size="small"
                  required
                  autoFocus
                />
                {isOn('middle_name') && (
                  <TextField
                    label={t('contacts.middleName')}
                    value={profileValues.middle_name}
                    onChange={(e) => onProfileValueChange({ ...profileValues, middle_name: e.target.value })}
                    size="small"
                  />
                )}
                <TextField
                  label={t('contactDetail.lastname')}
                  value={profileValues.lastname}
                  onChange={(e) => onProfileValueChange({ ...profileValues, lastname: e.target.value })}
                  size="small"
                />
                {isOn('suffix') && (
                  <TextField
                    label={t('contacts.suffix')}
                    value={profileValues.suffix}
                    onChange={(e) => onProfileValueChange({ ...profileValues, suffix: e.target.value })}
                    size="small"
                  />
                )}
                <TextField
                  label={t('contactDetail.nickname')}
                  value={profileValues.nickname}
                  onChange={(e) => onProfileValueChange({ ...profileValues, nickname: e.target.value })}
                  size="small"
                />
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, mt: 1 }}>
                  <Typography variant="overline" color="text.secondary" sx={{ letterSpacing: 0.08, fontSize: '0.72rem' }}>
                    {t('contactDetail.section.metadata')}
                  </Typography>
                </Box>
                <TextField
                  select
                  label={t('contactDetail.kind')}
                  value={profileValues.kind}
                  onChange={(e) => onProfileValueChange({ ...profileValues, kind: e.target.value })}
                  size="small"
                >
                  <MenuItem value="human">{t('contactDetail.human')}</MenuItem>
                  <MenuItem value="animal">{t('contactDetail.animal')}</MenuItem>
                </TextField>
                {isOn('cardKind') && (
                  <TextField
                    select
                    label={t('contacts.cardKindLabel')}
                    value={profileValues.cardKind}
                    onChange={(e) => onProfileValueChange({ ...profileValues, cardKind: e.target.value })}
                    size="small"
                  >
                    <MenuItem value="">
                      <em>{t('common.none')}</em>
                    </MenuItem>
                    {(['individual', 'group', 'org', 'location', 'application', 'device'] as const).map((opt) => (
                      <MenuItem key={opt} value={opt}>
                        {t(`contacts.cardKind.${opt}`)}
                      </MenuItem>
                    ))}
                  </TextField>
                )}
                {isOn('language') && (
                  <LanguageField
                    value={profileValues.language}
                    onChange={(v) => onProfileValueChange({ ...profileValues, language: v })}
                    size="small"
                    fullWidth
                  />
                )}
                <Box sx={{ display: 'flex', gap: 1, justifyContent: 'flex-end' }}>
                  <Box sx={{ display: 'flex', gap: 1 }}>
                    <IconButton size="small" color="primary" onClick={onSaveProfile} aria-label={t('common.save')}>
                      <SaveIcon />
                    </IconButton>
                    <IconButton size="small" onClick={onCancelEditProfile} aria-label={t('common.cancel')}>
                      <CloseIcon />
                    </IconButton>
                  </Box>
                </Box>
              </Box>
            ) : (
              // View Mode
              <>
                {archived && (
                  // T102/T62: neutral, matching the same badge on the contacts
                  // list. Archived is a state, not a warning condition.
                  <Chip
                    label={t('contactDetail.archivedBadge')}
                    color="default"
                    size="small"
                    sx={{ mb: 1 }}
                  />
                )}
                {compactActions ? (
                  // T54: on narrow viewports the single MoreVertIcon menu button
                  // is absolutely positioned so it stays pinned to the upper-right
                  // corner regardless of how many lines the name wraps to.
                  <Box
                    sx={{
                      position: 'relative',
                      '&:hover .edit-icon': {
                        opacity: 1
                      }
                    }}
                  >
                    <Box sx={{ display: 'flex', alignItems: 'center', pr: 5 }}>
                      <Typography variant="h5" sx={{ fontWeight: 500, lineHeight: 1.2, overflowWrap: 'anywhere' }}>
                        {displayName}
                      </Typography>
                      {isMe && (
                        // T90: being yourself is not a status condition — neutral
                        // chip per T62.
                        <Chip
                          label={t('contactDetail.youBadge')}
                          color="default"
                          size="small"
                          sx={{ ml: 1, flexShrink: 0 }}
                        />
                      )}
                      {onToggleFavorite && (
                        // Issue #173: always-visible star toggle.
                        <IconButton
                          size="small"
                          color="primary"
                          onClick={onToggleFavorite}
                          aria-label={record.is_favorite ? t('contactDetail.unfavoriteContact') : t('contactDetail.favoriteContact')}
                          sx={{ ml: 0.5, flexShrink: 0 }}
                        >
                          {record.is_favorite ? <StarIcon fontSize="small" /> : <StarBorderIcon fontSize="small" />}
                        </IconButton>
                      )}
                      <IconButton
                        className="edit-icon"
                        size="small"
                        color="primary"
                        onClick={onStartEditProfile}
                        sx={{
                          ml: 1,
                          opacity: 0,
                          transition: 'opacity 0.2s',
                          flexShrink: 0,
                        }}
                      >
                        <EditIcon fontSize="small" />
                      </IconButton>
                    </Box>
                    <Box
                      sx={{
                        position: 'absolute',
                        top: 0,
                        right: 0,
                      }}
                    >
                      <IconButton
                        size="small"
                        aria-label={t('contactDetail.actions')}
                        onClick={(e) => setActionsMenuAnchor(e.currentTarget)}
                      >
                        <MoreVertIcon />
                      </IconButton>
                      <Menu
                        anchorEl={actionsMenuAnchor}
                        open={actionsMenuOpen}
                        onClose={() => setActionsMenuAnchor(null)}
                      >
                        {archived ? (
                          onUnarchiveContact && (
                            <MenuItem key="unarchive" onClick={() => { setActionsMenuAnchor(null); onUnarchiveContact(); }}>
                              <ListItemIcon><UnarchiveIcon fontSize="small" /></ListItemIcon>
                              <ListItemText>{t('contactDetail.unarchive')}</ListItemText>
                            </MenuItem>
                          )
                        ) : (
                          [
                            onToggleMe && (
                              <MenuItem key="toggle-me" onClick={() => { setActionsMenuAnchor(null); onToggleMe(); }}>
                                <ListItemIcon><PersonIcon fontSize="small" /></ListItemIcon>
                                <ListItemText>{isMe ? t('contactDetail.unmarkAsMe') : t('contactDetail.markAsMe')}</ListItemText>
                              </MenuItem>
                            ),
                            onStayInTouch && (
                              <MenuItem key="stay-in-touch" onClick={() => { setActionsMenuAnchor(null); onStayInTouch(); }}>
                                <ListItemIcon><AutoModeIcon fontSize="small" /></ListItemIcon>
                                <ListItemText>{t('contactDetail.stayInTouch')}</ListItemText>
                              </MenuItem>
                            ),
                            onMergeContact && (
                              <MenuItem key="merge" onClick={() => { setActionsMenuAnchor(null); onMergeContact(); }}>
                                <ListItemIcon><MergeIcon fontSize="small" /></ListItemIcon>
                                <ListItemText>{t('contactMerge.mergeButton')}</ListItemText>
                              </MenuItem>
                            ),
                            onPrepView && (
                              <MenuItem key="prep" onClick={() => { setActionsMenuAnchor(null); onPrepView(); }}>
                                <ListItemIcon><SvgIcon fontSize="small"><path d={mdiNoteMultipleOutline} /></SvgIcon></ListItemIcon>
                                <ListItemText>{t('prep.title')}</ListItemText>
                              </MenuItem>
                            ),
                            onShareContact && (
                              <MenuItem key="share" onClick={() => { setActionsMenuAnchor(null); onShareContact(); }}>
                                <ListItemIcon><ShareIcon fontSize="small" /></ListItemIcon>
                                <ListItemText>{t('contactShares.shareDialog.title')}</ListItemText>
                              </MenuItem>
                            ),
                            <MenuItem key="vcf4" onClick={() => { setActionsMenuAnchor(null); onExportContact('vcf4'); }}>
                              <ListItemIcon><SvgIcon fontSize="small"><path d={mdiDownloadOutline} /></SvgIcon></ListItemIcon>
                              <ListItemText>vCard 4.0</ListItemText>
                            </MenuItem>,
                            <MenuItem key="vcf3" onClick={() => { setActionsMenuAnchor(null); onExportContact('vcf3'); }}>
                              <ListItemIcon><SvgIcon fontSize="small"><path d={mdiDownloadOutline} /></SvgIcon></ListItemIcon>
                              <ListItemText>vCard 3.0</ListItemText>
                            </MenuItem>,
                            <MenuItem key="jscontact" onClick={() => { setActionsMenuAnchor(null); onExportContact('jscontact'); }}>
                              <ListItemIcon><SvgIcon fontSize="small"><path d={mdiDownloadOutline} /></SvgIcon></ListItemIcon>
                              <ListItemText>JSContact</ListItemText>
                            </MenuItem>,
                            onArchiveContact && (
                              <MenuItem key="archive" onClick={() => { setActionsMenuAnchor(null); onArchiveContact(); }}>
                                <ListItemIcon><ArchiveIcon fontSize="small" /></ListItemIcon>
                                <ListItemText>{t('contactDetail.archive')}</ListItemText>
                              </MenuItem>
                            ),
                            onDeleteContact && (
                              <MenuItem key="delete" onClick={() => { setActionsMenuAnchor(null); onDeleteContact(); }}>
                                <ListItemIcon><DeleteIcon fontSize="small" color="error" /></ListItemIcon>
                                <ListItemText sx={{ color: 'error.main' }}>{t('contactDetail.deleteContact')}</ListItemText>
                              </MenuItem>
                            ),
                          ]
                        )}
                      </Menu>
                    </Box>
                  </Box>
                ) : (
                  // On wide viewports, the buttons sit next to the name in normal
                  // flex flow. T54 keeps this layout unchanged — the flex-wrap here
                  // on wide screens rarely causes the wrap the ticket is about, and
                  // inline buttons cannot sensibly overlay the name the way a single
                  // icon can on narrow widths.
                  <Box
                    sx={{
                      display: 'flex',
                      alignItems: 'flex-start',
                      justifyContent: 'space-between',
                      flexWrap: 'wrap',
                      gap: 0.5,
                      '&:hover .edit-icon': {
                        opacity: 1
                      }
                    }}
                  >
                    <Box sx={{ display: 'flex', alignItems: 'center' }}>
                      <Typography variant="h5" sx={{ fontWeight: 500, lineHeight: 1.2, overflowWrap: 'anywhere' }}>
                        {displayName}
                      </Typography>
                      {isMe && (
                        // T90: see the compact-layout badge above.
                        <Chip
                          label={t('contactDetail.youBadge')}
                          color="default"
                          size="small"
                          sx={{ ml: 1, flexShrink: 0 }}
                        />
                      )}
                      {onToggleFavorite && (
                        // Issue #173: see the compact-layout star above.
                        <IconButton
                          size="small"
                          color="primary"
                          onClick={onToggleFavorite}
                          aria-label={record.is_favorite ? t('contactDetail.unfavoriteContact') : t('contactDetail.favoriteContact')}
                          sx={{ ml: 0.5, flexShrink: 0 }}
                        >
                          {record.is_favorite ? <StarIcon fontSize="small" /> : <StarBorderIcon fontSize="small" />}
                        </IconButton>
                      )}
                      <IconButton
                        className="edit-icon"
                        size="small"
                        color="primary"
                        onClick={onStartEditProfile}
                        sx={{
                          ml: 1,
                          opacity: 0,
                          transition: 'opacity 0.2s',
                          flexShrink: 0,
                        }}
                      >
                        <EditIcon fontSize="small" />
                      </IconButton>
                    </Box>
                    <Box sx={{ display: 'flex', gap: 1 }}>
                      {archived ? (
                        onUnarchiveContact && (
                          // T102: no explicit color. Archive/unarchive are
                          // reversible state changes, so they take the neutral
                          // outlined treatment (matching BulkActionsBar);
                          // warning/error stay reserved for destructive or
                          // irreversible actions like delete.
                          <Button
                            variant="outlined"
                            size="small"
                            startIcon={<UnarchiveIcon />}
                            onClick={onUnarchiveContact}
                          >
                            {t('contactDetail.unarchive')}
                          </Button>
                        )
                      ) : (
                        <>
                          {onStayInTouch && (
                            <Button
                              variant="outlined"
                              size="small"
                              startIcon={<AutoModeIcon />}
                              onClick={onStayInTouch}
                            >
                              {t('contactDetail.stayInTouch')}
                            </Button>
                          )}
                          {onMergeContact && (
                            <Button
                              variant="outlined"
                              size="small"
                              startIcon={<MergeIcon />}
                              onClick={onMergeContact}
                            >
                              {t('contactMerge.mergeButton')}
                            </Button>
                          )}
                          {onPrepView && (
                            <Button
                              variant="outlined"
                              size="small"
                              startIcon={<SvgIcon><path d={mdiNoteMultipleOutline} /></SvgIcon>}
                              onClick={onPrepView}
                            >
                              {t('prep.title')}
                            </Button>
                          )}
                          {onShareContact && (
                            <Button
                              variant="outlined"
                              size="small"
                              startIcon={<ShareIcon />}
                              onClick={onShareContact}
                            >
                              {t('contactShares.shareDialog.title')}
                            </Button>
                          )}
                          <Button
                            variant="outlined"
                            size="small"
                            startIcon={<SvgIcon><path d={mdiDownloadOutline} /></SvgIcon>}
                            onClick={(e) => setExportMenuAnchor(e.currentTarget)}
                          >
                            {t('contactDetail.exportVCard')}
                          </Button>
                          <Menu
                            anchorEl={exportMenuAnchor}
                            open={exportMenuOpen}
                            onClose={() => setExportMenuAnchor(null)}
                          >
                            <MenuItem onClick={() => { setExportMenuAnchor(null); onExportContact('vcf4'); }}>
                              <ListItemText>vCard 4.0</ListItemText>
                            </MenuItem>
                            <MenuItem onClick={() => { setExportMenuAnchor(null); onExportContact('vcf3'); }}>
                              <ListItemText>vCard 3.0</ListItemText>
                            </MenuItem>
                            <MenuItem onClick={() => { setExportMenuAnchor(null); onExportContact('jscontact'); }}>
                              <ListItemText>JSContact</ListItemText>
                            </MenuItem>
                          </Menu>
                          {onArchiveContact && (
                            // T102: see the unarchive button above.
                            <Button
                              variant="outlined"
                              size="small"
                              startIcon={<ArchiveIcon />}
                              onClick={onArchiveContact}
                            >
                              {t('contactDetail.archive')}
                            </Button>
                          )}
                          <Button
                            variant="outlined"
                            size="small"
                            color="error"
                            startIcon={<DeleteIcon />}
                            onClick={onDeleteContact}
                          >
                            {t('contactDetail.delete')}
                          </Button>
                        </>
                      )}
                    </Box>
                  </Box>
                )}
                {kind === 'animal' && (
                  <Typography variant="body2" color="text.secondary" sx={{ mt: 0.25 }}>
                    {t(`contactDetail.${kind}`)}
                  </Typography>
                )}
              </>
            )}

            {/* Circles Section */}
            <Box sx={{ mt: 1, '&:hover .edit-icon': { opacity: 1 } }}>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, mb: 0.5 }}>
                <GroupIcon fontSize="small" color="action" />
                <Typography variant="caption" color="text.secondary">
                  {t('contactDetail.circles')}
                </Typography>
                {/* T89: ml:1 + flexShrink:0, matching the Name field's pencil
                    below -- ml:'auto' pushed this to the far right edge of the
                    header card, hundreds of px from the label and chips it
                    edits on a wide viewport. */}
                <IconButton className="edit-icon" size="small" color="primary" onClick={() => setEditingCircles(!editingCircles)}
                  sx={{ ml: 1, flexShrink: 0, opacity: 0, transition: 'opacity 0.2s' }}>
                  <EditIcon fontSize="small" />
                </IconButton>
              </Box>

              {editingCircles ? (
                <Box>
                  <Stack direction="row" spacing={1} flexWrap="wrap" sx={{ gap: 1, mb: 1 }}>
                    {contactCircles.length > 0 ? (
                      contactCircles.map((c) => (
                        <Chip key={c.id} label={c.name} size="small" icon={<GroupIcon />}
                          onDelete={() => onRemoveCircle(c)} />
                      ))
                    ) : (
                      <Typography variant="caption" color="text.secondary">{t('contactDetail.noCircles')}</Typography>
                    )}
                  </Stack>
                  <Stack direction="row" spacing={1} flexWrap="wrap" sx={{ gap: 1, mt: 1 }}>
                    <Autocomplete
                      size="small"
                      options={allCircles.filter(c => !contactCircles.find(cc => cc.id === c.id))}
                      getOptionLabel={(c) => c.name}
                      value={null}
                      onChange={(_, value) => { if (value) onAddCircle(value); }}
                      blurOnSelect
                      sx={{ minWidth: { xs: '100%', sm: 200 } }}
                      renderInput={(params) => (
                        <TextField {...params} label={t('contacts.selectCircle')} size="small" />
                      )}
                    />
                    <TextField size="small" placeholder={t('contactDetail.newCircle')}
                      value={newCircleName} onChange={(e) => setNewCircleName(e.target.value)}
                      onKeyDown={(e) => { if (e.key === 'Enter' && newCircleName.trim()) { onAddCircle({ id: '', created_at: '', updated_at: '', name: newCircleName.trim() }); setNewCircleName(''); }}}
                      sx={{ flexGrow: 1, minWidth: 0 }} />
                    <Button size="small" variant="contained" disabled={!newCircleName.trim()}
                      onClick={() => { onAddCircle({ id: '', created_at: '', updated_at: '', name: newCircleName.trim() }); setNewCircleName(''); }}>
                      {t('contactDetail.add')}
                    </Button>
                  </Stack>
                </Box>
              ) : (
                <Stack direction="row" spacing={1} flexWrap="wrap" sx={{ gap: 1 }}>
                  {contactCircles.length > 0 ? (
                    contactCircles.map((c) => (
                      <Chip key={c.id} label={c.name} size="small" icon={<GroupIcon />} />
                    ))
                  ) : (
                    <Typography variant="caption" color="text.secondary">{t('contactDetail.noCircles')}</Typography>
                  )}
                </Stack>
              )}
            </Box>

            {/* Tags Section */}
            <Box sx={{ mt: 1.5, '&:hover .edit-icon': { opacity: 1 } }}>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, mb: 0.5 }}>
                <LocalOfferIcon fontSize="small" color="action" />
                <Typography variant="caption" color="text.secondary">
                  {t('contactDetail.tags')}
                </Typography>
                {/* T89: see the circles pencil above. */}
                <IconButton className="edit-icon" size="small" color="primary" onClick={() => setEditingTags(!editingTags)}
                  sx={{ ml: 1, flexShrink: 0, opacity: 0, transition: 'opacity 0.2s' }}>
                  <EditIcon fontSize="small" />
                </IconButton>
              </Box>

              {editingTags ? (
                <Box>
                  <Stack direction="row" spacing={1} flexWrap="wrap" sx={{ gap: 1, mb: 1 }}>
                    {contactTags.length > 0 ? (
                      contactTags.map((t) => (
                        <Chip key={t.id} label={t.name} size="small" icon={<LocalOfferIcon />}
                          onDelete={() => onRemoveTag(t)} />
                      ))
                    ) : (
                      <Typography variant="caption" color="text.secondary">{t('contactDetail.noTags')}</Typography>
                    )}
                  </Stack>
                  <Stack direction="row" spacing={1} flexWrap="wrap" sx={{ gap: 1, mt: 1 }}>
                    <Autocomplete
                      size="small"
                      options={allTags.filter(t => !contactTags.find(ct => ct.id === t.id))}
                      getOptionLabel={(t) => t.name}
                      value={null}
                      onChange={(_, value) => { if (value) onAddTag(value); }}
                      blurOnSelect
                      sx={{ minWidth: { xs: '100%', sm: 200 } }}
                      renderInput={(params) => (
                        <TextField {...params} label={t('contactDetail.selectTag')} size="small" />
                      )}
                    />
                    <TextField size="small" placeholder={t('contactDetail.newTag')}
                      value={newTagName} onChange={(e) => setNewTagName(e.target.value)}
                      onKeyDown={(e) => { if (e.key === 'Enter' && newTagName.trim()) { onAddTag({ id: '', created_at: '', updated_at: '', name: newTagName.trim() }); setNewTagName(''); }}}
                      sx={{ flexGrow: 1, minWidth: 0 }} />
                    <Button size="small" variant="contained" disabled={!newTagName.trim()}
                      onClick={() => { onAddTag({ id: '', created_at: '', updated_at: '', name: newTagName.trim() }); setNewTagName(''); }}>
                      {t('contactDetail.add')}
                    </Button>
                  </Stack>
                </Box>
              ) : (
                <Stack direction="row" spacing={1} flexWrap="wrap" sx={{ gap: 1 }}>
                  {contactTags.length > 0 ? (
                    contactTags.map((t) => (
                      <Chip key={t.id} label={t.name} size="small" icon={<LocalOfferIcon />} />
                    ))
                  ) : (
                    <Typography variant="caption" color="text.secondary">{t('contactDetail.noTags')}</Typography>
                  )}
                </Stack>
              )}
            </Box>
          </Box>
        </Box>
      </CardContent>
    </Card>
  );
}
