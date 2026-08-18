import { useState, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import {
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Box,
  Typography,
  Stepper,
  Step,
  StepLabel,
  Avatar,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Select,
  MenuItem,
  FormControl,
  FormHelperText,
  Chip,
  Alert,
  LinearProgress,
  IconButton,
  TablePagination,
  CircularProgress,
} from '@mui/material';
import AppDialog from './AppDialog';
import CloudUploadIcon from '@mui/icons-material/CloudUpload';
import CheckCircleIcon from '@mui/icons-material/CheckCircle';
import WarningIcon from '@mui/icons-material/Warning';
import ErrorIcon from '@mui/icons-material/Error';
import CloseIcon from '@mui/icons-material/Close';
import { useSnackbar } from '../context/SnackbarContext';
import { getErrorMessage } from '../utils/errorHandler';
import {
  uploadCSVForImport,
  uploadVCFForImport,
  getImportPreview,
  confirmImport,
  confirmVCFImport,
  ColumnMapping,
  ImportUploadResponse,
  ImportPreviewResponse,
  RowImportAction,
  ImportResult,
  ImportRowPreview,
  ImportMergeDiff,
  IMPORTABLE_CONTACT_FIELDS,
  CONTACT_FIELD_LABELS,
  REPEATABLE_VALUE_FIELDS,
} from '../api/import';

interface ImportContactsDialogProps {
  open: boolean;
  onClose: () => void;
  onImportComplete: () => void;
}

type ImportStep = 'upload' | 'mapping' | 'preview' | 'result';
type ImportType = 'csv' | 'vcf';

const CSV_STEP_KEYS = ['upload', 'mapColumns', 'review', 'done'] as const;
const VCF_STEP_KEYS = ['upload', 'review', 'done'] as const;

// T56: the preview table is paginated client-side so a full address-book
// import (hundreds of rows) stays usable instead of mounting one Select per
// row.
const PREVIEW_PAGE_SIZE = 20;

export default function ImportContactsDialog({
  open,
  onClose,
  onImportComplete,
}: ImportContactsDialogProps) {
  const { t } = useTranslation();
  const { showSuccess } = useSnackbar();

  // Step state
  const [activeStep, setActiveStep] = useState(0);
  const [step, setStep] = useState<ImportStep>('upload');
  const [importType, setImportType] = useState<ImportType>('csv');

  // Upload state
  const [uploadResponse, setUploadResponse] = useState<ImportUploadResponse | null>(null);
  const [mappings, setMappings] = useState<ColumnMapping[]>([]);

  // Preview state
  const [previewResponse, setPreviewResponse] = useState<ImportPreviewResponse | null>(null);
  const [rowActions, setRowActions] = useState<Map<number, string>>(new Map());
  const [previewPage, setPreviewPage] = useState(0);

  // Result state
  const [importResult, setImportResult] = useState<ImportResult | null>(null);

  // UI state
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [dragOver, setDragOver] = useState(false);

  // Reset dialog state
  const resetDialog = useCallback(() => {
    setActiveStep(0);
    setStep('upload');
    setImportType('csv');
    setUploadResponse(null);
    setMappings([]);
    setPreviewResponse(null);
    setRowActions(new Map());
    setPreviewPage(0);
    setImportResult(null);
    setLoading(false);
    setError(null);
    setDragOver(false);
  }, []);

  // Handle dialog close
  const handleClose = () => {
    resetDialog();
    onClose();
  };

  // Handle file upload
  const handleFileUpload = async (file: File) => {
    const fileName = file.name.toLowerCase();
    const isCSV = fileName.endsWith('.csv');
    const isVCF = fileName.endsWith('.vcf');

    if (!isCSV && !isVCF) {
      setError(t('contacts.import.errors.invalidFile', 'Please select a valid CSV or VCF file'));
      return;
    }

    // Keep in sync with backend/services/import_service.go's MaxVCFSize /
    // MaxCSVSize (T56 raised them for full address-book imports).
    const maxSize = isVCF ? 50 * 1024 * 1024 : 20 * 1024 * 1024; // 50MB VCF, 20MB CSV
    if (file.size > maxSize) {
      setError(t('contacts.import.errors.fileTooLarge', 'File is too large. Maximum size is {{size}}MB', {
        size: maxSize / (1024 * 1024),
      }));
      return;
    }

    setLoading(true);
    setError(null);

    try {
      if (isVCF) {
        // VCF import - goes directly to preview (no mapping needed)
        setImportType('vcf');
        const response = await uploadVCFForImport(file);
        setPreviewResponse(response);
        setPreviewPage(0);

        // Initialize row actions based on suggested actions
        const initialActions = new Map<number, string>();
        response.rows.forEach((row) => {
          initialActions.set(row.row_index, row.suggested_action);
        });
        setRowActions(initialActions);

        setStep('preview');
        setActiveStep(1); // VCF skips mapping, so preview is step 1
      } else {
        // CSV import - needs column mapping
        setImportType('csv');
        const response = await uploadCSVForImport(file);
        setUploadResponse(response);
        setMappings(response.suggested_mappings);
        setStep('mapping');
        setActiveStep(1);
      }
    } catch (err) {
      setError(getErrorMessage(err));
    } finally {
      setLoading(false);
    }
  };

  // Handle file input change
  const handleFileInputChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (file) {
      handleFileUpload(file);
    }
    // Reset input value to allow re-selecting the same file
    event.target.value = '';
  };

  // Handle drag and drop
  const handleDragOver = (event: React.DragEvent) => {
    event.preventDefault();
    setDragOver(true);
  };

  const handleDragLeave = () => {
    setDragOver(false);
  };

  const handleDrop = (event: React.DragEvent) => {
    event.preventDefault();
    setDragOver(false);
    const file = event.dataTransfer.files?.[0];
    if (file) {
      handleFileUpload(file);
    }
  };

  // Handle mapping change
  const handleMappingChange = (index: number, field: string) => {
    const newMappings = [...mappings];
    newMappings[index] = { ...newMappings[index], contact_field: field };
    setMappings(newMappings);
  };

  // Handle preview generation
  const handleGeneratePreview = async () => {
    if (!uploadResponse) return;

    // Check if at least one field is mapped
    const hasMappings = mappings.some((m) => m.contact_field !== '');
    if (!hasMappings) {
      setError(t('contacts.import.errors.noMappings', 'Please map at least one column'));
      return;
    }

    setLoading(true);
    setError(null);

    try {
      const response = await getImportPreview(uploadResponse.session_id, mappings);
      setPreviewResponse(response);
      setPreviewPage(0);

      // Initialize row actions based on suggested actions
      const initialActions = new Map<number, string>();
      response.rows.forEach((row) => {
        initialActions.set(row.row_index, row.suggested_action);
      });
      setRowActions(initialActions);

      setStep('preview');
      setActiveStep(2);
    } catch (err) {
      setError(getErrorMessage(err));
    } finally {
      setLoading(false);
    }
  };

  // Handle row action change
  const handleRowActionChange = (rowIndex: number, action: string) => {
    const newActions = new Map(rowActions);
    newActions.set(rowIndex, action);
    setRowActions(newActions);
  };

  // T56 bulk controls: apply one action across the whole preview in a single
  // step so a full address-book import doesn't need per-row clicks.
  const handleAcceptAll = () => {
    if (!previewResponse) return;
    const newActions = new Map(rowActions);
    previewResponse.rows.forEach((row) => {
      // Errors are disabled in the UI and always stay skip; every other row
      // takes its own suggested action (add for new, update for duplicates,
      // skip for within-batch duplicates).
      if (row.validation_errors.length === 0) newActions.set(row.row_index, row.suggested_action);
    });
    setRowActions(newActions);
  };

  const handleSkipAll = () => {
    if (!previewResponse) return;
    const newActions = new Map(rowActions);
    previewResponse.rows.forEach((row) => {
      if (row.validation_errors.length === 0) newActions.set(row.row_index, 'skip');
    });
    setRowActions(newActions);
  };

  // Handle import confirmation
  const handleConfirmImport = async () => {
    if (!previewResponse) return;

    setLoading(true);
    setError(null);

    try {
      const actions: RowImportAction[] = [];
      rowActions.forEach((action, rowIndex) => {
        actions.push({ row_index: rowIndex, action: action as 'skip' | 'add' | 'update' });
      });

      // Use appropriate confirm endpoint based on import type
      const result = importType === 'vcf'
        ? await confirmVCFImport(previewResponse.session_id, actions)
        : await confirmImport(previewResponse.session_id, actions);

      setImportResult(result);
      setStep('result');
      setActiveStep(importType === 'vcf' ? 2 : 3); // VCF has fewer steps

      if (result.created > 0 || result.updated > 0) {
        showSuccess(
          t('contacts.import.result.success', 'Import completed: {{created}} created, {{updated}} updated', {
            created: result.created,
            updated: result.updated,
          })
        );
        onImportComplete();
      }
    } catch (err) {
      setError(getErrorMessage(err));
    } finally {
      setLoading(false);
    }
  };

  // Calculate summary counts
  const getSummaryCounts = () => {
    let toCreate = 0;
    let toUpdate = 0;
    let toSkip = 0;

    rowActions.forEach((action) => {
      if (action === 'add') toCreate++;
      else if (action === 'update') toUpdate++;
      else toSkip++;
    });

    return { toCreate, toUpdate, toSkip };
  };

  // Render upload step
  const renderUploadStep = () => (
    <Box sx={{ py: 4 }}>
      <Box
        sx={{
          border: '2px dashed',
          borderColor: dragOver ? 'primary.main' : 'grey.400',
          borderRadius: 2,
          p: 6,
          textAlign: 'center',
          cursor: 'pointer',
          bgcolor: dragOver ? 'action.hover' : 'background.paper',
          transition: 'all 0.2s',
        }}
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
        onClick={() => document.getElementById('import-file-input')?.click()}
      >
        <input
          id="import-file-input"
          type="file"
          accept=".csv,.vcf"
          style={{ display: 'none' }}
          aria-label={t('contacts.import.upload.dragDrop')}
          onChange={handleFileInputChange}
        />
        <CloudUploadIcon sx={{ fontSize: 48, color: 'grey.500', mb: 2 }} />
        <Typography variant="h6" gutterBottom>
          {t('contacts.import.upload.dragDrop', 'Drag and drop a CSV or VCF file here, or click to select')}
        </Typography>
        <Typography variant="body2" color="text.secondary">
          {t('contacts.import.upload.supportedFormats', 'Supported formats: CSV (spreadsheet), VCF (vCard)')}
        </Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
          {t('contacts.import.upload.maxSize', 'Maximum file size: 50MB (VCF) / 20MB (CSV)')}
        </Typography>
      </Box>
    </Box>
  );

  // Render mapping step
  const renderMappingStep = () => {
    if (!uploadResponse) return null;

    // "field#group" keys of single-slot targets mapped by more than one column. Repeatable
    // multi-value fields (email/phone/website/IM) are excluded — multiple columns there
    // legitimately become separate entries.
    const slotCounts = new Map<string, number>();
    mappings.forEach((m) => {
      if (!m.contact_field || REPEATABLE_VALUE_FIELDS.has(m.contact_field)) return;
      const key = `${m.contact_field}#${m.group}`;
      slotCounts.set(key, (slotCounts.get(key) ?? 0) + 1);
    });
    const conflictKeys = new Set<string>();
    slotCounts.forEach((count, key) => {
      if (count > 1) conflictKeys.add(key);
    });
    const mappingKey = (m: ColumnMapping) => `${m.contact_field}#${m.group}`;
    const conflictLabels = Array.from(
      new Set(
        mappings
          .filter((m) => m.contact_field && conflictKeys.has(mappingKey(m)))
          .map((m) => CONTACT_FIELD_LABELS[m.contact_field] || m.contact_field)
      )
    );

    return (
      <Box>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          {t(
            'contacts.import.mapping.description',
            "Match your CSV columns to contact fields. Columns marked 'Ignore' will not be imported."
          )}
        </Typography>
        {conflictLabels.length > 0 && (
          <Alert severity="warning" sx={{ mb: 2 }}>
            {t(
              'contacts.import.mapping.duplicateWarning',
              'These single-value fields are mapped to more than one column; only the last column will be used: {{fields}}',
              { fields: conflictLabels.join(', ') }
            )}
          </Alert>
        )}
        <TableContainer component={Paper} sx={{ maxHeight: 400 }}>
          <Table stickyHeader size="small">
            <TableHead>
              <TableRow>
                <TableCell>{t('contacts.import.mapping.csvColumn', 'CSV Column')}</TableCell>
                <TableCell>{t('contacts.import.mapping.sampleData', 'Sample')}</TableCell>
                <TableCell>{t('contacts.import.mapping.mapsTo', 'Maps To')}</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {mappings.map((mapping, index) => {
                const isConflict =
                  !!mapping.contact_field && conflictKeys.has(mappingKey(mapping));
                return (
                  <TableRow key={index}>
                    <TableCell>
                      <Typography variant="body2" fontWeight="medium">
                        {mapping.csv_column}
                      </Typography>
                    </TableCell>
                    <TableCell>
                      <Typography variant="body2" color="text.secondary" noWrap sx={{ maxWidth: 150 }}>
                        {uploadResponse.sample_data[0]?.[index] || '-'}
                      </Typography>
                    </TableCell>
                    <TableCell>
                      <FormControl size="small" sx={{ minWidth: 180 }} error={isConflict}>
                        <Select
                          value={mapping.contact_field}
                          onChange={(e) => handleMappingChange(index, e.target.value)}
                          displayEmpty
                        >
                          <MenuItem value="">
                            <em>{t('contacts.import.mapping.ignore', '-- Ignore --')}</em>
                          </MenuItem>
                          {IMPORTABLE_CONTACT_FIELDS.map((field) => (
                            <MenuItem key={field} value={field}>
                              {CONTACT_FIELD_LABELS[field] || field}
                            </MenuItem>
                          ))}
                        </Select>
                        {isConflict && (
                          <FormHelperText>
                            {t('contacts.import.mapping.duplicateField', 'Mapped more than once')}
                          </FormHelperText>
                        )}
                      </FormControl>
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </TableContainer>
      </Box>
    );
  };

  // Render preview step
  const renderPreviewStep = () => {
    if (!previewResponse) return null;

    const { toCreate, toUpdate, toSkip } = getSummaryCounts();
    const errorCount = previewResponse.rows.filter((r) => r.validation_errors.length > 0).length;

    // T96: how many duplicate/within-batch rows still sit on their seeded
    // default action, i.e. how many conflicts remain to be consciously
    // resolved ("Resolve Conflicts (N remaining)").
    const conflictRows = previewResponse.rows.filter((r) => r.validation_errors.length === 0 && isConflictRow(r));
    const conflictsRemaining = conflictRows.filter(
      (r) => (rowActions.get(r.row_index) ?? r.suggested_action) === r.suggested_action
    ).length;

    // T56: client-side page of the preview rows. Rows whose index lands on
    // the current page are the only ones mounted, so a full address-book
    // import never mounts hundreds of cards at once.
    const pageStart = previewPage * PREVIEW_PAGE_SIZE;
    const pageRows = previewResponse.rows.slice(pageStart, pageStart + PREVIEW_PAGE_SIZE);

    return (
      <Box>
        {/* Summary */}
        <Box sx={{ mb: 2, display: 'flex', gap: 1, flexWrap: 'wrap', alignItems: 'center' }}>
          <Chip
            icon={<CheckCircleIcon />}
            label={t('contacts.import.preview.toCreate', '{{count}} to create', { count: toCreate })}
            color="success"
            variant="outlined"
          />
          <Chip
            icon={<WarningIcon />}
            label={t('contacts.import.preview.toUpdate', '{{count}} to update', { count: toUpdate })}
            color="warning"
            variant="outlined"
          />
          <Chip
            label={t('contacts.import.preview.toSkip', '{{count}} to skip', { count: toSkip })}
            variant="outlined"
          />
          {errorCount > 0 && (
            <Chip
              icon={<ErrorIcon />}
              label={t('contacts.import.preview.errors', '{{count}} with errors', { count: errorCount })}
              color="error"
              variant="outlined"
            />
          )}
          <Box sx={{ flexGrow: 1 }} />
          <Button size="small" onClick={handleAcceptAll}>
            {t('contacts.import.preview.resolveAllAsMerged', 'Resolve all as merged')}
          </Button>
          <Button size="small" onClick={handleSkipAll}>
            {t('contacts.import.preview.skipAll', 'Skip all')}
          </Button>
        </Box>

        {/* T96 conflict heading. The "no matches" copy is only accurate when
            there were never any conflicts: once one exists, even a fully
            resolved set is not "everything below will be added as new". */}
        {conflictsRemaining > 0 ? (
          <Typography variant="subtitle2" sx={{ mb: 1 }}>
            {t('contacts.import.preview.resolveConflicts', 'Resolve Conflicts ({{count}} remaining)', {
              count: conflictsRemaining,
            })}
          </Typography>
        ) : conflictRows.length > 0 ? (
          <Typography variant="subtitle2" sx={{ mb: 1 }}>
            {t('contacts.import.preview.allResolved', 'All conflicts resolved — review the decisions below.')}
          </Typography>
        ) : (
          <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
            {t('contacts.import.preview.noConflicts', 'No duplicate matches — everything below will be added as new.')}
          </Typography>
        )}

        {/* Per-row decision cards */}
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1.5, maxHeight: 420, overflowY: 'auto' }}>
          {pageRows.map((row) => (
            <ImportRowCard
              key={row.row_index}
              row={row}
              action={rowActions.get(row.row_index) ?? row.suggested_action}
              onActionChange={(action) => handleRowActionChange(row.row_index, action)}
            />
          ))}
        </Box>
        {previewResponse.rows.length > PREVIEW_PAGE_SIZE && (
          <TablePagination
            component="div"
            count={previewResponse.rows.length}
            page={previewPage}
            rowsPerPage={PREVIEW_PAGE_SIZE}
            rowsPerPageOptions={[PREVIEW_PAGE_SIZE]}
            onPageChange={(_, newPage) => setPreviewPage(newPage)}
          />
        )}
      </Box>
    );
  };

  // Render result step
  const renderResultStep = () => {
    if (!importResult) return null;

    return (
      <Box sx={{ py: 2 }}>
        <Alert severity="success" sx={{ mb: 2 }}>
          {t('contacts.import.result.title', 'Import Complete')}
        </Alert>

        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
          <Typography>
            <CheckCircleIcon color="success" sx={{ verticalAlign: 'middle', mr: 1 }} />
            {t('contacts.import.result.created', '{{count}} contacts created', {
              count: importResult.created,
            })}
          </Typography>
          <Typography>
            <WarningIcon color="warning" sx={{ verticalAlign: 'middle', mr: 1 }} />
            {t('contacts.import.result.updated', '{{count}} contacts updated', {
              count: importResult.updated,
            })}
          </Typography>
          <Typography color="text.secondary">
            {t('contacts.import.result.skipped', '{{count}} rows skipped', {
              count: importResult.skipped,
            })}
          </Typography>
        </Box>

        {importResult.errors.length > 0 && (
          <Box sx={{ mt: 2 }}>
            <Typography variant="subtitle2" color="error" gutterBottom>
              {t('contacts.import.result.errors', 'Errors')}:
            </Typography>
            {importResult.errors.map((error, index) => (
              <Typography key={index} variant="body2" color="error">
                {error}
              </Typography>
            ))}
          </Box>
        )}
      </Box>
    );
  };

  // Render current step content
  const renderStepContent = () => {
    switch (step) {
      case 'upload':
        return renderUploadStep();
      case 'mapping':
        return renderMappingStep();
      case 'preview':
        return renderPreviewStep();
      case 'result':
        return renderResultStep();
      default:
        return null;
    }
  };

  // Render action buttons
  const renderActions = () => {
    switch (step) {
      case 'upload':
        return (
          <Button onClick={handleClose}>{t('common.cancel', 'Cancel')}</Button>
        );
      case 'mapping':
        return (
          <>
            <Button onClick={() => { setStep('upload'); setActiveStep(0); }}>
              {t('common.back', 'Back')}
            </Button>
            <Button variant="contained" onClick={handleGeneratePreview} disabled={loading}>
              {t('common.continue', 'Continue')}
            </Button>
          </>
        );
      case 'preview':
        return (
          <>
            <Button
              onClick={() => {
                if (importType === 'vcf') {
                  // VCF goes back to upload (no mapping step)
                  setStep('upload');
                  setActiveStep(0);
                  setPreviewResponse(null);
                  setRowActions(new Map());
                  setPreviewPage(0);
                } else {
                  // CSV goes back to mapping
                  setStep('mapping');
                  setActiveStep(1);
                }
              }}
              disabled={loading}
            >
              {t('common.back', 'Back')}
            </Button>
            <Button onClick={handleClose} disabled={loading}>{t('common.cancel', 'Cancel')}</Button>
            <Button
              variant="contained"
              onClick={handleConfirmImport}
              disabled={loading}
              startIcon={loading ? <CircularProgress size={16} color="inherit" /> : undefined}
            >
              {loading
                ? t('contacts.import.preview.importing', 'Importing…')
                : t('contacts.import.preview.applyDecisions', 'Apply Decisions ({{count}})', {
                    count: previewResponse?.rows.filter((r) => r.validation_errors.length === 0).length ?? 0,
                  })}
            </Button>
          </>
        );
      case 'result':
        return (
          <Button variant="contained" onClick={handleClose}>
            {t('contacts.import.result.done', 'Done')}
          </Button>
        );
      default:
        return null;
    }
  };

  return (
    <AppDialog open={open} onClose={handleClose} maxWidth="md" fullWidth>
      <DialogTitle>
        <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
          {t('contacts.import.title', 'Import Contacts')}
          <IconButton onClick={handleClose} size="small" aria-label={t('common.close')}>
            <CloseIcon />
          </IconButton>
        </Box>
      </DialogTitle>

      <DialogContent dividers>
        {/* Stepper - different steps for CSV vs VCF */}
        <Stepper activeStep={activeStep} sx={{ mb: 3 }}>
          {(importType === 'vcf' ? VCF_STEP_KEYS : CSV_STEP_KEYS).map((key) => (
            <Step key={key}>
              <StepLabel>{t(`contacts.import.steps.${key}`)}</StepLabel>
            </Step>
          ))}
        </Stepper>

        {/* Loading indicator */}
        {loading && <LinearProgress sx={{ mb: 2 }} />}

        {/* Error alert */}
        {error && (
          <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)}>
            {error}
          </Alert>
        )}

        {/* Step content */}
        {renderStepContent()}
      </DialogContent>

      <DialogActions>{renderActions()}</DialogActions>
    </AppDialog>
  );
}

// --- T96 review-step helpers (T96) -------------------------------

// isConflictRow reports whether a row needs a conscious decision: it matches an
// existing record (duplicate_match) or duplicates an earlier row of the same
// import (batch_duplicate_of).
function isConflictRow(row: ImportRowPreview): boolean {
  return !!row.duplicate_match || row.batch_duplicate_of !== null;
}

const IMPORT_DIFF_KIND_LABELS: Record<string, string> = {
  email: 'email',
  phone: 'phone',
  address: 'address',
  url: 'website',
  impp: 'IM',
};

// ImportMergeDiffSummary renders what "Merge" will change on the matched
// existing contact: appended entries (+ new phone: ...) and overwritten
// scalars (Job Title: A → B). The backend always sends updated/added as arrays
// (never null — trap #8), but a stray null from a proxy/older server must not
// crash the render, so both are guarded with ?? [].
function ImportMergeDiffSummary({ diff }: { diff: ImportMergeDiff }) {
  const { t } = useTranslation();
  const updated = diff.updated ?? [];
  const added = diff.added ?? [];
  if (updated.length === 0 && added.length === 0) {
    return (
      <Typography variant="caption" color="text.secondary" display="block">
        {t('contacts.import.preview.noDiff', 'No changes — the records already match.')}
      </Typography>
    );
  }
  return (
    <Box sx={{ mt: 0.5 }}>
      <Typography variant="caption" sx={{ fontWeight: 600 }}>
        {t('contacts.import.preview.diffTitle', 'Will merge:')}
      </Typography>
      {added.map((a, i) => (
        <Typography key={`added-${i}`} variant="caption" display="block" color="text.secondary">
          {t('contacts.import.preview.diffAdded', '+ new {{kind}}: {{value}}', {
            kind: t(`contacts.import.preview.kind.${a.kind}`, IMPORT_DIFF_KIND_LABELS[a.kind] || a.kind),
            value: a.value,
          })}
        </Typography>
      ))}
      {updated.map((u, i) => (
        <Typography key={`updated-${i}`} variant="caption" display="block" color="text.secondary">
          {t('contacts.import.preview.diffUpdated', '{{label}}: {{old}} → {{new}}', {
            label: u.label,
            old: u.old || '—',
            new: u.new,
          })}
        </Typography>
      ))}
    </Box>
  );
}

// ImportRowCard is one contact's decision card in the review step: name,
// match/diff summary, and the Merge / Keep Both / Discard New choice. Merge is
// only offered when the row matched an EXISTING record — a within-batch
// duplicate (batch_duplicate_of without a DB match) has nothing to merge into.
function ImportRowCard({
  row,
  action,
  onActionChange,
}: {
  row: ImportRowPreview;
  action: string;
  onActionChange: (action: 'skip' | 'add' | 'update') => void;
}) {
  const { t } = useTranslation();
  const hasErrors = row.validation_errors.length > 0;
  const name =
    [row.parsed_contact.firstname, row.parsed_contact.lastname].filter(Boolean).join(' ').trim() ||
    t('contacts.import.preview.unnamed', 'Unnamed');
  const sub = row.parsed_contact.email || row.parsed_contact.phone || '';
  const canMerge = !hasErrors && !!row.duplicate_match;

  const reasonLabel = (reason: string) => {
    switch (reason) {
      case 'email':
        return t('duplicates.reason.email');
      case 'name':
        return t('duplicates.reason.name');
      case 'phone':
        return t('duplicates.reason.phone');
      default:
        return reason;
    }
  };

  return (
    <Box
      sx={{
        p: 1.5,
        border: '1px solid',
        borderColor: 'divider',
        borderRadius: 1,
        bgcolor: 'background.paper',
      }}
    >
      <Box sx={{ display: 'flex', gap: 1.5, alignItems: 'flex-start' }}>
        <Avatar sx={{ width: 36, height: 36, bgcolor: 'primary.main', fontSize: '1rem', flexShrink: 0 }}>
          {(row.parsed_contact.firstname || '?').charAt(0)}
        </Avatar>
        <Box sx={{ minWidth: 0, flex: 1 }}>
          <Typography variant="body2" sx={{ fontWeight: 500 }}>
            {name}
          </Typography>
          {sub && (
            <Typography variant="caption" color="text.secondary">
              {sub}
            </Typography>
          )}

          {hasErrors ? (
            <Box>
              {row.validation_errors.map((e, i) => (
                <Typography key={i} variant="caption" color="error" display="block">
                  {e}
                </Typography>
              ))}
            </Box>
          ) : row.duplicate_match ? (
            <Typography variant="caption" sx={{ color: 'warning.main' }} display="block">
              {t('contacts.import.preview.duplicateOf', 'Matches: {{name}} ({{reason}})', {
                name: [row.duplicate_match.existing_firstname, row.duplicate_match.existing_lastname]
                  .filter(Boolean)
                  .join(' ')
                  .trim(),
                reason: reasonLabel(row.duplicate_match.match_reason),
              })}
            </Typography>
          ) : row.batch_duplicate_of !== null ? (
            <Typography variant="caption" sx={{ color: 'warning.main' }} display="block">
              {t('contacts.import.preview.batchDuplicateOf', 'Duplicates row {{row}} of this import', {
                row: (row.batch_duplicate_of ?? 0) + 1,
              })}
            </Typography>
          ) : (
            <Typography variant="caption" sx={{ color: 'success.main' }} display="block">
              {t('contacts.import.preview.newContact', 'New contact — no match found')}
            </Typography>
          )}

          {!hasErrors && row.merge_diff && <ImportMergeDiffSummary diff={row.merge_diff} />}
        </Box>
      </Box>
      <Box sx={{ display: 'flex', gap: 1, mt: 1, flexWrap: 'wrap' }}>
        <Button
          size="small"
          variant={action === 'update' ? 'contained' : 'outlined'}
          onClick={() => onActionChange('update')}
          disabled={!canMerge}
          aria-pressed={action === 'update'}
        >
          {t('contacts.import.preview.actionMerge', 'Merge')}
        </Button>
        <Button
          size="small"
          variant={action === 'add' ? 'contained' : 'outlined'}
          onClick={() => onActionChange('add')}
          disabled={hasErrors}
          aria-pressed={action === 'add'}
        >
          {t('contacts.import.preview.actionKeepBoth', 'Keep Both')}
        </Button>
        <Button
          size="small"
          variant={action === 'skip' ? 'contained' : 'outlined'}
          color={action === 'skip' ? 'inherit' : undefined}
          onClick={() => onActionChange('skip')}
          disabled={hasErrors}
          aria-pressed={action === 'skip'}
        >
          {t('contacts.import.preview.actionDiscard', 'Discard New')}
        </Button>
      </Box>
    </Box>
  );
}
