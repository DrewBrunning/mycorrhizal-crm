import { Box, Card, CardContent, Avatar, Typography, Chip, IconButton, Stack, TextField, Autocomplete, Button, SvgIcon, Menu, MenuItem, ListItemText, ListItemIcon, useTheme, useMediaQuery } from '@mui/material';
import { useState } from 'react';
import MoreVertIcon from '@mui/icons-material/MoreVert';
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
import { mdiDownloadOutline } from '@mdi/js';
import { useTranslation } from 'react-i18next';
import { ContactFieldKey, resolveEnabledFields } from '../contactFields';
import { ContactRecordResponse, nameComponentValue } from '../api/contacts';
import { Circle } from '../api/circles';
import { Tag } from '../api/tags';

export interface ProfileValues {
  prefix: string;
  firstname: string;
  middle_name: string;
  lastname: string;
  suffix: string;
  nickname: string;
  gender: string;
  // CRMEnvelope.Kind (T27): human|animal.
  kind: string;
  // Card.Kind (WP13, T29): individual|group|org|location|application|device.
  cardKind: string;
  // Card.Language (WP4, T29): default language tag.
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
  onMergeContact?: () => void;
  onExportContact: (format: string) => void;
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
  onMergeContact,
  onExportContact
}: ContactHeaderProps) {
  const { t } = useTranslation();
  const enabled = enabledFields ?? resolveEnabledFields(null);
  const [exportMenuAnchor, setExportMenuAnchor] = useState<HTMLElement | null>(null);
  const exportMenuOpen = Boolean(exportMenuAnchor);
  // Overflow menu for the header action buttons on narrow viewports (T28).
  const [actionsMenuAnchor, setActionsMenuAnchor] = useState<HTMLElement | null>(null);
  const actionsMenuOpen = Boolean(actionsMenuAnchor);
  // Collapse the text buttons (stay in touch / merge / export / archive) into
  // a single overflow menu at phone widths where they cannot fit beside the
  // name. The edit pencil stays visible at every size.
  const theme = useTheme();
  const compactActions = useMediaQuery(theme.breakpoints.down('sm'));
  const isOn = (key: ContactFieldKey) => enabled.has(key);

  const card = record.card || {};
  const prefix = nameComponentValue(card.name?.components, 'title');
  const firstname = nameComponentValue(card.name?.components, 'given') || '';
  const middleName = nameComponentValue(card.name?.components, 'given2');
  const lastname = nameComponentValue(card.name?.components, 'surname') || '';
  const suffix = nameComponentValue(card.name?.components, 'generation');
  const nickname = card.nicknames?.[0]?.name;
  const gender = record.gender;
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
                <Autocomplete
                  freeSolo
                  options={['male', 'female', 'other', 'prefer_not_to_say']}
                  getOptionLabel={(v) => ['male', 'female', 'other', 'prefer_not_to_say'].includes(v) ? t(`contactDetail.${v}`) : (v || '')}
                  value={profileValues.gender || null}
                  onChange={(_, v) => onProfileValueChange({ ...profileValues, gender: v || '' })}
                  onInputChange={(_, v) => onProfileValueChange({ ...profileValues, gender: v })}
                  size="small"
                  renderInput={(params) => (
                    <TextField {...params} label={t('contactDetail.gender')} fullWidth />
                  )}
                />
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
                  <TextField
                    label={t('contacts.language')}
                    value={profileValues.language}
                    onChange={(e) => onProfileValueChange({ ...profileValues, language: e.target.value })}
                    size="small"
                    placeholder="en"
                  />
                )}
                <Box sx={{ display: 'flex', gap: 1, justifyContent: 'space-between' }}>
                  <IconButton
                    size="small"
                    color="error"
                    onClick={onDeleteContact}
                    title={t('contactDetail.deleteContact')}
                  >
                    <DeleteIcon />
                  </IconButton>
                  <Box sx={{ display: 'flex', gap: 1 }}>
                    <IconButton size="small" color="primary" onClick={onSaveProfile}>
                      <SaveIcon />
                    </IconButton>
                    <IconButton size="small" onClick={onCancelEditProfile}>
                      <CloseIcon />
                    </IconButton>
                  </Box>
                </Box>
              </Box>
            ) : (
              // View Mode
              <>
                {archived && (
                  <Chip
                    label={t('contactDetail.archivedBadge')}
                    color="warning"
                    size="small"
                    sx={{ mb: 1 }}
                  />
                )}
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
                    <IconButton
                      className="edit-icon"
                      size="small"
                      onClick={onStartEditProfile}
                      sx={{
                        ml: 1,
                        opacity: 0,
                        transition: 'opacity 0.2s'
                      }}
                    >
                      <EditIcon fontSize="small" />
                    </IconButton>
                  </Box>
                  <Box sx={{ display: 'flex', gap: 1 }}>
                    {compactActions ? (
                      <>
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
                            ]
                          )}
                        </Menu>
                      </>
                    ) : (
                      archived ? (
                        onUnarchiveContact && (
                          <Button
                            variant="outlined"
                            size="small"
                            color="success"
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
                            <Button
                              variant="outlined"
                              size="small"
                              color="warning"
                              startIcon={<ArchiveIcon />}
                              onClick={onArchiveContact}
                            >
                              {t('contactDetail.archive')}
                            </Button>
                          )}
                        </>
                      )
                    )}
                  </Box>
                </Box>
                {gender && (
                  <Typography variant="body2" color="text.secondary" sx={{ mt: 0.25 }}>
                    {['male', 'female', 'other', 'prefer_not_to_say'].includes(gender) ? t(`contactDetail.${gender}`) : gender}
                  </Typography>
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
                <IconButton className="edit-icon" size="small" onClick={() => setEditingCircles(!editingCircles)}
                  sx={{ ml: 'auto', opacity: 0, transition: 'opacity 0.2s' }}>
                  <EditIcon fontSize="small" />
                </IconButton>
              </Box>

              {editingCircles ? (
                <Box>
                  <Stack direction="row" spacing={1} flexWrap="wrap" sx={{ gap: 1, mb: 1 }}>
                    {contactCircles.length > 0 ? (
                      contactCircles.map((c) => (
                        <Chip key={c.id} label={c.name} size="small" icon={<GroupIcon />} color="primary"
                          onDelete={() => onRemoveCircle(c)} />
                      ))
                    ) : (
                      <Typography variant="caption" color="text.secondary">{t('contactDetail.noCircles')}</Typography>
                    )}
                  </Stack>
                  <Stack direction="row" spacing={1} sx={{ mt: 1 }}>
                    <Autocomplete
                      size="small"
                      options={allCircles.filter(c => !contactCircles.find(cc => cc.id === c.id))}
                      getOptionLabel={(c) => c.name}
                      value={null}
                      onChange={(_, value) => { if (value) onAddCircle(value); }}
                      blurOnSelect
                      sx={{ minWidth: 200 }}
                      renderInput={(params) => (
                        <TextField {...params} label={t('contacts.selectCircle')} size="small" />
                      )}
                    />
                    <TextField size="small" placeholder={t('contactDetail.newCircle')}
                      value={newCircleName} onChange={(e) => setNewCircleName(e.target.value)}
                      onKeyDown={(e) => { if (e.key === 'Enter' && newCircleName.trim()) { onAddCircle({ id: '', created_at: '', updated_at: '', name: newCircleName.trim() }); setNewCircleName(''); }}}
                      sx={{ flexGrow: 1 }} />
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
                      <Chip key={c.id} label={c.name} size="small" icon={<GroupIcon />} color="primary" />
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
                <IconButton className="edit-icon" size="small" onClick={() => setEditingTags(!editingTags)}
                  sx={{ ml: 'auto', opacity: 0, transition: 'opacity 0.2s' }}>
                  <EditIcon fontSize="small" />
                </IconButton>
              </Box>

              {editingTags ? (
                <Box>
                  <Stack direction="row" spacing={1} flexWrap="wrap" sx={{ gap: 1, mb: 1 }}>
                    {contactTags.length > 0 ? (
                      contactTags.map((t) => (
                        <Chip key={t.id} label={t.name} size="small" icon={<LocalOfferIcon />} color="secondary"
                          onDelete={() => onRemoveTag(t)} />
                      ))
                    ) : (
                      <Typography variant="caption" color="text.secondary">{t('contactDetail.noTags')}</Typography>
                    )}
                  </Stack>
                  <Stack direction="row" spacing={1} sx={{ mt: 1 }}>
                    <Autocomplete
                      size="small"
                      options={allTags.filter(t => !contactTags.find(ct => ct.id === t.id))}
                      getOptionLabel={(t) => t.name}
                      value={null}
                      onChange={(_, value) => { if (value) onAddTag(value); }}
                      blurOnSelect
                      sx={{ minWidth: 200 }}
                      renderInput={(params) => (
                        <TextField {...params} label={t('contactDetail.selectTag')} size="small" />
                      )}
                    />
                    <TextField size="small" placeholder={t('contactDetail.newTag')}
                      value={newTagName} onChange={(e) => setNewTagName(e.target.value)}
                      onKeyDown={(e) => { if (e.key === 'Enter' && newTagName.trim()) { onAddTag({ id: '', created_at: '', updated_at: '', name: newTagName.trim() }); setNewTagName(''); }}}
                      sx={{ flexGrow: 1 }} />
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
                      <Chip key={t.id} label={t.name} size="small" icon={<LocalOfferIcon />} color="secondary" />
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
