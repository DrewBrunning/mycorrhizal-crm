// Issue #1482: request-contract + error-mapping tests for the thin src/api
// modules, driven through msw so every call runs the real apiFetch wrapper.
// Each row asserts (a) the request that goes on the wire -- method, full URL
// including query string, JSON body -- and (b) that a 403 backend envelope is
// mapped to the module's typed error (ApiError for parseErrorResponse-based
// modules, a plain Error carrying the envelope message for handleResponse-based
// ones). Modules with richer behavior have their own *.test.ts.
import { HttpResponse, http, type JsonBodyType } from 'msw';
import { describe, expect, test } from 'vitest';
import { type Captured, errorEnvelope, mockApi, server, setupMswServer } from '../test/mswServer';
import * as admin from './admin';
import * as auth from './auth';
import * as cadence from './cadencePolicies';
import { API_BASE_URL, ApiError } from './client';
import * as merge from './contactMerge';
import * as shares from './contactShares';
import * as syncConflicts from './contactSyncConflicts';
import * as dashboard from './dashboard';
import * as decay from './dataDecayPolicies';
import * as suggestions from './dataSuggestions';
import * as duplicates from './duplicates';
import * as fieldDefs from './fieldDefinitions';
import * as gifts from './gifts';
import * as graph from './graph';
import * as health from './health';
import * as jobRuns from './jobRuns';
import * as linkTypes from './linkFieldTypes';
import * as meerkat from './meerkatImport';
import * as monica from './monicaImport';
import * as mycorrhizal from './mycorrhizalImport';
import * as notifications from './notifications';
import * as occasionEvents from './occasionEvents';
import * as prefs from './preferences';
import * as reachOut from './reachOutSuggestions';
import * as search from './search';
import * as sessions from './sessions';
import * as sourceImport from './sourceImport';
import * as systemStatus from './systemStatus';
import * as webhooks from './webhooks';

setupMswServer();

type Method = 'get' | 'post' | 'put' | 'patch' | 'delete';

interface Row {
  name: string;
  call: () => Promise<unknown>;
  method: Method;
  /** Route registered with msw (no query string). */
  route: string;
  /** Expected path+query as sent; defaults to `/api/v1${route}`. */
  url?: string;
  /** Expected JSON body; omit to assert no body was sent. */
  body?: unknown;
  ok?: JsonBodyType | Response;
  result?: unknown;
  /** 'plain' for handleResponse-based modules. Default 'api'. */
  err?: 'api' | 'plain';
}

const NO_CONTENT = () => new Response(null, { status: 204 });

const rows: Row[] = [
  // --- admin
  {
    name: 'admin.getCurrentUser',
    call: () => admin.getCurrentUser(),
    method: 'get',
    route: '/users/me',
    ok: { id: 1 },
    result: { id: 1 },
  },
  {
    name: 'admin.getUsers defaults',
    call: () => admin.getUsers(),
    method: 'get',
    route: '/admin/users',
    url: '/api/v1/admin/users?page=1&limit=25',
  },
  {
    name: 'admin.getUsers explicit',
    call: () => admin.getUsers(3, 10),
    method: 'get',
    route: '/admin/users',
    url: '/api/v1/admin/users?page=3&limit=10',
  },
  {
    name: 'admin.createUser',
    call: () => admin.createUser({ username: 'u' } as never),
    method: 'post',
    route: '/admin/users',
    body: { username: 'u' },
  },
  {
    name: 'admin.updateUser',
    call: () => admin.updateUser(4, { email: 'e' } as never),
    method: 'patch',
    route: '/admin/users/4',
    body: { email: 'e' },
  },
  {
    name: 'admin.triggerReminders',
    call: () => admin.triggerReminders(),
    method: 'post',
    route: '/admin/trigger-reminders',
    ok: NO_CONTENT(),
  },
  {
    name: 'admin.deleteUser',
    call: () => admin.deleteUser(4),
    method: 'delete',
    route: '/admin/users/4',
    ok: NO_CONTENT(),
  },
  {
    name: 'admin.resetUserTwoFactor',
    call: () => admin.resetUserTwoFactor(4),
    method: 'post',
    route: '/admin/users/4/reset-2fa',
    ok: { id: 4 },
    result: { id: 4 },
  },

  // --- auth (handleResponse based)
  {
    name: 'auth.requestPasswordReset',
    call: () => auth.requestPasswordReset('a@b.c'),
    method: 'post',
    route: '/password-reset/request',
    body: { email: 'a@b.c' },
    ok: { message: 'sent' },
    result: 'sent',
    err: 'plain',
  },
  {
    name: 'auth.requestPasswordReset default message',
    call: () => auth.requestPasswordReset('a@b.c'),
    method: 'post',
    route: '/password-reset/request',
    body: { email: 'a@b.c' },
    result: 'If an account exists, password reset instructions were sent.',
    err: 'plain',
  },
  {
    name: 'auth.confirmPasswordReset',
    call: () => auth.confirmPasswordReset('tok', 'pw'),
    method: 'post',
    route: '/password-reset/confirm',
    body: { token: 'tok', password: 'pw' },
    ok: { message: 'done' },
    result: 'done',
    err: 'plain',
  },
  {
    name: 'auth.confirmPasswordReset default message',
    call: () => auth.confirmPasswordReset('tok', 'pw'),
    method: 'post',
    route: '/password-reset/confirm',
    body: { token: 'tok', password: 'pw' },
    result: 'Password reset successful.',
    err: 'plain',
  },
  {
    name: 'auth.changePassword',
    call: () => auth.changePassword('old', 'new'),
    method: 'post',
    route: '/users/change-password',
    body: { current_password: 'old', new_password: 'new' },
    ok: { message: 'changed' },
    result: 'changed',
    err: 'plain',
  },
  {
    name: 'auth.changePassword default message',
    call: () => auth.changePassword('old', 'new'),
    method: 'post',
    route: '/users/change-password',
    body: { current_password: 'old', new_password: 'new' },
    result: 'Password updated successfully.',
    err: 'plain',
  },

  // --- cadence policies
  {
    name: 'cadence.getCadencePolicies',
    call: () => cadence.getCadencePolicies('uid 1'),
    method: 'get',
    route: '/cadence-policies',
    url: '/api/v1/cadence-policies?entity_id=uid+1',
    ok: { cadence_policies: [] },
    result: { cadence_policies: [] },
  },
  {
    name: 'cadence.getOverdueCadences',
    call: () => cadence.getOverdueCadences(),
    method: 'get',
    route: '/cadence-policies/overdue',
    ok: { overdue: [] },
    result: { overdue: [] },
  },
  {
    name: 'cadence.createCadencePolicy unwraps cadence_policy',
    call: () => cadence.createCadencePolicy({ entity_id: 'u', target_interval_days: 30 }),
    method: 'post',
    route: '/cadence-policies',
    body: { entity_id: 'u', target_interval_days: 30 },
    ok: { message: 'm', cadence_policy: { id: 'p' } },
    result: { id: 'p' },
  },
  {
    name: 'cadence.updateCadencePolicy returns raw',
    call: () =>
      cadence.updateCadencePolicy('p1', {
        entity_id: 'u',
        target_interval_days: 7,
        qualifying_types: ['call'],
      }),
    method: 'put',
    route: '/cadence-policies/p1',
    body: { entity_id: 'u', target_interval_days: 7, qualifying_types: ['call'] },
    ok: { id: 'p1' },
    result: { id: 'p1' },
  },
  {
    name: 'cadence.deleteCadencePolicy',
    call: () => cadence.deleteCadencePolicy('p1'),
    method: 'delete',
    route: '/cadence-policies/p1',
    ok: NO_CONTENT(),
  },

  // --- contact merge
  {
    name: 'merge.previewContactMerge',
    call: () => merge.previewContactMerge(1, 2),
    method: 'post',
    route: '/contacts/merge/preview',
    body: { keep_id: 1, merge_id: 2 },
    ok: { keep_id: 1 },
    result: { keep_id: 1 },
  },
  {
    name: 'merge.commitContactMerge',
    call: () => merge.commitContactMerge(1, 2, { email: 'keeper' }),
    method: 'post',
    route: '/contacts/merge',
    body: { keep_id: 1, merge_id: 2, resolutions: { email: 'keeper' } },
    ok: { message: 'merged', contact: {} },
    result: { message: 'merged', contact: {} },
  },

  // --- contact shares
  {
    name: 'shares.getUserDirectory unwraps users',
    call: () => shares.getUserDirectory(),
    method: 'get',
    route: '/users/directory',
    ok: { users: [{ id: 1, username: 'a' }] },
    result: [{ id: 1, username: 'a' }],
  },
  {
    name: 'shares.createContactShare unwraps contact_share',
    call: () => shares.createContactShare({ to_user_id: 2, vcard_uid: 'v', sections: ['x'] }),
    method: 'post',
    route: '/contact-shares',
    body: { to_user_id: 2, vcard_uid: 'v', sections: ['x'] },
    ok: { contact_share: { id: 's' } },
    result: { id: 's' },
  },
  {
    name: 'shares.getIncomingContactShares no params',
    call: () => shares.getIncomingContactShares(),
    method: 'get',
    route: '/contact-shares/incoming',
    url: '/api/v1/contact-shares/incoming',
  },
  {
    name: 'shares.getIncomingContactShares cursor+limit',
    call: () => shares.getIncomingContactShares({ cursor: 'c', limit: 5 }),
    method: 'get',
    route: '/contact-shares/incoming',
    url: '/api/v1/contact-shares/incoming?cursor=c&limit=5',
  },
  {
    name: 'shares.getOutgoingContactShares no params',
    call: () => shares.getOutgoingContactShares(),
    method: 'get',
    route: '/contact-shares/outgoing',
    url: '/api/v1/contact-shares/outgoing',
  },
  {
    name: 'shares.getOutgoingContactShares cursor+limit',
    call: () => shares.getOutgoingContactShares({ cursor: 'c', limit: 5 }),
    method: 'get',
    route: '/contact-shares/outgoing',
    url: '/api/v1/contact-shares/outgoing?cursor=c&limit=5',
  },
  {
    name: 'shares.acceptContactShare',
    call: () => shares.acceptContactShare('s1'),
    method: 'post',
    route: '/contact-shares/s1/accept',
    ok: { session_id: 'x' },
    result: { session_id: 'x' },
  },
  {
    name: 'shares.confirmContactShare',
    call: () => shares.confirmContactShare('s1', 'sess', [{ row_index: 0, action: 'add' }]),
    method: 'post',
    route: '/contact-shares/s1/confirm',
    body: { session_id: 'sess', actions: [{ row_index: 0, action: 'add' }] },
    ok: { created: 1 },
    result: { created: 1 },
  },
  {
    name: 'shares.declineContactShare',
    call: () => shares.declineContactShare('s1'),
    method: 'post',
    route: '/contact-shares/s1/decline',
    ok: NO_CONTENT(),
  },

  // --- sync conflicts
  {
    name: 'syncConflicts.getContactSyncConflicts',
    call: () => syncConflicts.getContactSyncConflicts(),
    method: 'get',
    route: '/contact-sync-conflicts',
    ok: { sync_conflicts: [] },
    result: { sync_conflicts: [] },
  },
  {
    name: 'syncConflicts.restore',
    call: () => syncConflicts.restoreContactSyncConflict('c1'),
    method: 'post',
    route: '/contact-sync-conflicts/c1/restore',
    ok: NO_CONTENT(),
  },
  {
    name: 'syncConflicts.dismiss',
    call: () => syncConflicts.dismissContactSyncConflict('c1'),
    method: 'post',
    route: '/contact-sync-conflicts/c1/dismiss',
    ok: NO_CONTENT(),
  },

  // --- dashboard
  {
    name: 'dashboard.getDashboard',
    call: () => dashboard.getDashboard(),
    method: 'get',
    route: '/dashboard',
    ok: { birthdays: [] },
    result: { birthdays: [] },
  },

  // --- data decay
  {
    name: 'decay.getDataDecayPolicies',
    call: () => decay.getDataDecayPolicies('u1'),
    method: 'get',
    route: '/data-decay-policies',
    url: '/api/v1/data-decay-policies?entity_id=u1',
    ok: { data_decay_policies: [] },
    result: { data_decay_policies: [] },
  },
  {
    name: 'decay.getOverdueDataDecayPolicies',
    call: () => decay.getOverdueDataDecayPolicies(),
    method: 'get',
    route: '/data-decay-policies/overdue',
    ok: { overdue: [] },
    result: { overdue: [] },
  },
  {
    name: 'decay.createDataDecayPolicy unwraps',
    call: () => decay.createDataDecayPolicy({ entity_id: 'u', interval_days: 90 }),
    method: 'post',
    route: '/data-decay-policies',
    body: { entity_id: 'u', interval_days: 90 },
    ok: { data_decay_policy: { id: 'd' } },
    result: { id: 'd' },
  },
  {
    name: 'decay.updateDataDecayPolicy raw',
    call: () =>
      decay.updateDataDecayPolicy('d1', { entity_id: 'u', interval_days: 30, active: false }),
    method: 'put',
    route: '/data-decay-policies/d1',
    body: { entity_id: 'u', interval_days: 30, active: false },
    ok: { id: 'd1' },
    result: { id: 'd1' },
  },
  {
    name: 'decay.deleteDataDecayPolicy',
    call: () => decay.deleteDataDecayPolicy('d1'),
    method: 'delete',
    route: '/data-decay-policies/d1',
    ok: NO_CONTENT(),
  },
  {
    name: 'decay.verifyDataDecayPolicy raw',
    call: () => decay.verifyDataDecayPolicy('d1'),
    method: 'post',
    route: '/data-decay-policies/d1/verify',
    ok: { id: 'd1' },
    result: { id: 'd1' },
  },

  // --- data suggestions
  {
    name: 'suggestions.suggestContactAddresses',
    call: () => suggestions.suggestContactAddresses(),
    method: 'post',
    route: '/contacts/address-suggestions',
    ok: { suggestions: [], total: 0 },
    result: { suggestions: [], total: 0 },
  },
  {
    name: 'suggestions.applyContactAddressSuggestion',
    call: () =>
      suggestions.applyContactAddressSuggestion({
        contact_vcard_uid: 'c',
        source_kind: 'household',
        source_id: 's',
        address_key: 'k',
      }),
    method: 'post',
    route: '/contacts/address-suggestions/apply',
    body: { contact_vcard_uid: 'c', source_kind: 'household', source_id: 's', address_key: 'k' },
    ok: NO_CONTENT(),
  },

  // --- duplicates
  {
    name: 'duplicates.getDuplicatePairs defaults',
    call: () => duplicates.getDuplicatePairs(),
    method: 'get',
    route: '/contacts/duplicates',
    url: '/api/v1/contacts/duplicates?page=1&limit=50',
    ok: { pairs: [], total: 0, page: 1, limit: 50 },
    result: { pairs: [], total: 0, page: 1, limit: 50 },
  },
  {
    name: 'duplicates.getDuplicatePairs explicit',
    call: () => duplicates.getDuplicatePairs({ page: 2, limit: 5 }),
    method: 'get',
    route: '/contacts/duplicates',
    url: '/api/v1/contacts/duplicates?page=2&limit=5',
    ok: { total: 0, page: 2, limit: 5 },
    result: { pairs: [], total: 0, page: 2, limit: 5 },
  },
  {
    name: 'duplicates.dismissDuplicatePair',
    call: () => duplicates.dismissDuplicatePair('a', 'b'),
    method: 'post',
    route: '/contacts/duplicates/dismiss',
    body: { uid_a: 'a', uid_b: 'b' },
    ok: NO_CONTENT(),
  },

  // --- field definitions
  {
    name: 'fieldDefs.getFieldDefinitions default limit',
    call: () => fieldDefs.getFieldDefinitions(),
    method: 'get',
    route: '/field-definitions',
    url: '/api/v1/field-definitions?limit=100',
  },
  {
    name: 'fieldDefs.getFieldDefinitions explicit limit',
    call: () => fieldDefs.getFieldDefinitions(7),
    method: 'get',
    route: '/field-definitions',
    url: '/api/v1/field-definitions?limit=7',
  },
  {
    name: 'fieldDefs.createFieldDefinition unwraps',
    call: () => fieldDefs.createFieldDefinition({ label: 'L', key: 'k', type: 'string' }),
    method: 'post',
    route: '/field-definitions',
    body: { label: 'L', key: 'k', type: 'string' },
    ok: { field_definition: { id: 'f' } },
    result: { id: 'f' },
  },
  {
    name: 'fieldDefs.updateFieldDefinition raw',
    call: () => fieldDefs.updateFieldDefinition('f1', { label: 'L', key: 'k', type: 'text' }),
    method: 'put',
    route: '/field-definitions/f1',
    body: { label: 'L', key: 'k', type: 'text' },
    ok: { id: 'f1' },
    result: { id: 'f1' },
  },
  {
    name: 'fieldDefs.deleteFieldDefinition',
    call: () => fieldDefs.deleteFieldDefinition('f1'),
    method: 'delete',
    route: '/field-definitions/f1',
    ok: NO_CONTENT(),
  },
  {
    name: 'fieldDefs.reorderFieldDefinitions',
    call: () => fieldDefs.reorderFieldDefinitions(['b', 'a']),
    method: 'put',
    route: '/field-definitions/reorder',
    body: { order: ['b', 'a'] },
    ok: { field_definitions: [{ id: 'b' }] },
    result: [{ id: 'b' }],
  },
  {
    name: 'fieldDefs.reorderFieldDefinitions empty fallback',
    call: () => fieldDefs.reorderFieldDefinitions([]),
    method: 'put',
    route: '/field-definitions/reorder',
    body: { order: [] },
    result: [],
  },
  {
    name: 'fieldDefs.getContactFieldValues',
    call: () => fieldDefs.getContactFieldValues(9),
    method: 'get',
    route: '/contacts/9/field-values',
    ok: { field_values: [{ id: 1 }] },
    result: [{ id: 1 }],
  },
  {
    name: 'fieldDefs.getContactFieldValues empty fallback',
    call: () => fieldDefs.getContactFieldValues('9'),
    method: 'get',
    route: '/contacts/9/field-values',
    result: [],
  },
  {
    name: 'fieldDefs.replaceContactFieldValues',
    call: () => fieldDefs.replaceContactFieldValues(9, [{ field_definition_id: 'f', value: 1 }]),
    method: 'put',
    route: '/contacts/9/field-values',
    body: { field_values: [{ field_definition_id: 'f', value: 1 }] },
    ok: { field_values: [{ id: 2 }] },
    result: [{ id: 2 }],
  },
  {
    name: 'fieldDefs.replaceContactFieldValues empty fallback',
    call: () => fieldDefs.replaceContactFieldValues('9', []),
    method: 'put',
    route: '/contacts/9/field-values',
    body: { field_values: [] },
    result: [],
  },

  // --- gifts
  {
    name: 'gifts.getGifts no params',
    call: () => gifts.getGifts(),
    method: 'get',
    route: '/gifts',
    url: '/api/v1/gifts?limit=100',
  },
  {
    name: 'gifts.getGifts all params',
    call: () => gifts.getGifts({ entityId: 'e', cursor: 'c', limit: 5 }),
    method: 'get',
    route: '/gifts',
    url: '/api/v1/gifts?limit=5&entity_id=e&cursor=c',
  },
  {
    name: 'gifts.createGift unwraps',
    call: () => gifts.createGift({ entity_id: 'e', description: 'd' }),
    method: 'post',
    route: '/gifts',
    body: { entity_id: 'e', description: 'd' },
    ok: { gift: { id: 'g' } },
    result: { id: 'g' },
  },
  {
    name: 'gifts.updateGift raw',
    call: () => gifts.updateGift('g1', { entity_id: 'e', description: 'd', date: null }),
    method: 'put',
    route: '/gifts/g1',
    body: { entity_id: 'e', description: 'd', date: null },
    ok: { id: 'g1' },
    result: { id: 'g1' },
  },
  {
    name: 'gifts.deleteGift',
    call: () => gifts.deleteGift('g1'),
    method: 'delete',
    route: '/gifts/g1',
    ok: NO_CONTENT(),
  },

  // --- graph
  {
    name: 'graph.getGraph',
    call: () => graph.getGraph(),
    method: 'get',
    route: '/graph',
    ok: { nodes: [] },
    result: { nodes: [] },
  },
  {
    name: 'graph.getConnections from only',
    call: () => graph.getConnections({ from: 'u' }),
    method: 'get',
    route: '/graph/connections',
    url: '/api/v1/graph/connections?from=u',
  },
  {
    name: 'graph.getConnections depth 0 is sent',
    call: () => graph.getConnections({ from: 'u', depth: 0, relation: 'brother' }),
    method: 'get',
    route: '/graph/connections',
    url: '/api/v1/graph/connections?from=u&depth=0&relation=brother',
  },

  // --- job runs
  {
    name: 'jobRuns.getJobRunHealth',
    call: () => jobRuns.getJobRunHealth(),
    method: 'get',
    route: '/admin/job-runs/health',
    ok: { jobs: [] },
    result: { jobs: [] },
  },
  {
    name: 'jobRuns.listJobRuns no params',
    call: () => jobRuns.listJobRuns(),
    method: 'get',
    route: '/admin/job-runs',
    url: '/api/v1/admin/job-runs',
  },
  {
    name: 'jobRuns.listJobRuns all params',
    call: () =>
      jobRuns.listJobRuns({ jobName: 'j', result: 'failure', since: 's', until: 'u', limit: 0 }),
    method: 'get',
    route: '/admin/job-runs',
    url: '/api/v1/admin/job-runs?job_name=j&result=failure&since=s&until=u&limit=0',
  },

  // --- link field types
  {
    name: 'linkTypes.getLinkFieldTypes',
    call: () => linkTypes.getLinkFieldTypes(),
    method: 'get',
    route: '/link-field-types',
    ok: { link_field_types: [{ id: 'a' }] },
    result: [{ id: 'a' }],
  },
  {
    name: 'linkTypes.getLinkFieldTypes empty fallback',
    call: () => linkTypes.getLinkFieldTypes(),
    method: 'get',
    route: '/link-field-types',
    result: [],
  },
  {
    name: 'linkTypes.createLinkFieldType unwraps',
    call: () =>
      linkTypes.createLinkFieldType({ name: 'n', protocol: 'p://{value}', category: 'other' }),
    method: 'post',
    route: '/link-field-types',
    body: { name: 'n', protocol: 'p://{value}', category: 'other' },
    ok: { link_field_type: { id: 'l' } },
    result: { id: 'l' },
  },
  {
    name: 'linkTypes.updateLinkFieldType raw',
    call: () =>
      linkTypes.updateLinkFieldType('l1', {
        name: 'n',
        protocol: '',
        category: 'social',
        position: 2,
      }),
    method: 'put',
    route: '/link-field-types/l1',
    body: { name: 'n', protocol: '', category: 'social', position: 2 },
    ok: { id: 'l1' },
    result: { id: 'l1' },
  },
  {
    name: 'linkTypes.deleteLinkFieldType',
    call: () => linkTypes.deleteLinkFieldType('l1'),
    method: 'delete',
    route: '/link-field-types/l1',
    ok: NO_CONTENT(),
  },
  {
    name: 'linkTypes.reorderLinkFieldTypes',
    call: () => linkTypes.reorderLinkFieldTypes(['b', 'a']),
    method: 'put',
    route: '/link-field-types/reorder',
    body: { order: ['b', 'a'] },
    ok: { link_field_types: [{ id: 'b' }] },
    result: [{ id: 'b' }],
  },
  {
    name: 'linkTypes.reorderLinkFieldTypes empty fallback',
    call: () => linkTypes.reorderLinkFieldTypes([]),
    method: 'put',
    route: '/link-field-types/reorder',
    body: { order: [] },
    result: [],
  },

  // --- meerkat / monica / mycorrhizal import (source-import wrappers)
  {
    name: 'meerkat.startMeerkatFetch with user',
    call: () => meerkat.startMeerkatFetch('s1', 4),
    method: 'post',
    route: '/contacts/import/meerkat/fetch',
    body: { session_id: 's1', source_user_id: 4 },
    ok: NO_CONTENT(),
  },
  {
    name: 'meerkat.startMeerkatFetch without user sends null',
    call: () => meerkat.startMeerkatFetch('s1'),
    method: 'post',
    route: '/contacts/import/meerkat/fetch',
    body: { session_id: 's1', source_user_id: null },
    ok: NO_CONTENT(),
  },
  {
    name: 'meerkat.getMeerkatImportStatus',
    call: () => meerkat.getMeerkatImportStatus('a b'),
    method: 'get',
    route: '/contacts/import/meerkat/status',
    url: '/api/v1/contacts/import/meerkat/status?session_id=a%20b',
    ok: { phase: 'ready' },
    result: { phase: 'ready' },
  },
  {
    name: 'meerkat.getMeerkatImportPreview',
    call: () => meerkat.getMeerkatImportPreview('s1'),
    method: 'get',
    route: '/contacts/import/meerkat/preview',
    url: '/api/v1/contacts/import/meerkat/preview?session_id=s1',
    ok: { rows: [] },
    result: { rows: [] },
  },
  {
    name: 'meerkat.confirmMeerkatImport',
    call: () => meerkat.confirmMeerkatImport('s1', [{ row_index: 1, action: 'skip' }]),
    method: 'post',
    route: '/contacts/import/meerkat/confirm',
    body: { session_id: 's1', actions: [{ row_index: 1, action: 'skip' }] },
    ok: NO_CONTENT(),
  },
  {
    name: 'monica.connectMonica',
    call: () => monica.connectMonica('https://m', 'tok'),
    method: 'post',
    route: '/contacts/import/monica/connect',
    body: { base_url: 'https://m', api_token: 'tok' },
    ok: { session_id: 's' },
    result: { session_id: 's' },
  },
  {
    name: 'monica.startMonicaFetch',
    call: () =>
      monica.startMonicaFetch('s1', { include_relationships: true, include_extras: false }),
    method: 'post',
    route: '/contacts/import/monica/fetch',
    body: { session_id: 's1', include_relationships: true, include_extras: false },
    ok: NO_CONTENT(),
  },
  {
    name: 'monica.getMonicaImportStatus',
    call: () => monica.getMonicaImportStatus('s1'),
    method: 'get',
    route: '/contacts/import/monica/status',
    url: '/api/v1/contacts/import/monica/status?session_id=s1',
    ok: { phase: 'ready' },
    result: { phase: 'ready' },
  },
  {
    name: 'monica.getMonicaImportPreview',
    call: () => monica.getMonicaImportPreview('s1'),
    method: 'get',
    route: '/contacts/import/monica/preview',
    url: '/api/v1/contacts/import/monica/preview?session_id=s1',
    ok: { rows: [] },
    result: { rows: [] },
  },
  {
    name: 'monica.confirmMonicaImport',
    call: () => monica.confirmMonicaImport('s1', []),
    method: 'post',
    route: '/contacts/import/monica/confirm',
    body: { session_id: 's1', actions: [] },
    ok: NO_CONTENT(),
  },
  {
    name: 'mycorrhizal.startMycorrhizalFetch',
    call: () => mycorrhizal.startMycorrhizalFetch('s1'),
    method: 'post',
    route: '/import/mycorrhizal/fetch',
    body: { session_id: 's1' },
    ok: NO_CONTENT(),
  },
  {
    name: 'mycorrhizal.getMycorrhizalImportStatus',
    call: () => mycorrhizal.getMycorrhizalImportStatus('s1'),
    method: 'get',
    route: '/import/mycorrhizal/status',
    url: '/api/v1/import/mycorrhizal/status?session_id=s1',
    ok: { phase: 'done' },
    result: { phase: 'done' },
  },
  {
    name: 'mycorrhizal.getMycorrhizalImportPreview',
    call: () => mycorrhizal.getMycorrhizalImportPreview('s1'),
    method: 'get',
    route: '/import/mycorrhizal/preview',
    url: '/api/v1/import/mycorrhizal/preview?session_id=s1',
    ok: { rows: [] },
    result: { rows: [] },
  },
  {
    name: 'mycorrhizal.confirmMycorrhizalImport',
    call: () => mycorrhizal.confirmMycorrhizalImport('s1', []),
    method: 'post',
    route: '/import/mycorrhizal/confirm',
    body: { session_id: 's1', actions: [] },
    ok: NO_CONTENT(),
  },

  // --- notifications
  {
    name: 'notifications.getNotificationConfig',
    call: () => notifications.getNotificationConfig(),
    method: 'get',
    route: '/notifications/config',
    ok: { ntfy_url: 'u' },
    result: { ntfy_url: 'u' },
  },
  {
    name: 'notifications.saveNotificationConfig',
    call: () => notifications.saveNotificationConfig({ ntfy_url: 'u', notify_push: true }),
    method: 'put',
    route: '/notifications/config',
    body: { ntfy_url: 'u', notify_push: true },
    ok: { ntfy_url: 'u' },
    result: { ntfy_url: 'u' },
  },
  {
    name: 'notifications.testNotificationChannel',
    call: () => notifications.testNotificationChannel('gotify'),
    method: 'post',
    route: '/notifications/config/test',
    body: { channel: 'gotify' },
    ok: { ok: false, error: 'blocked' },
    result: { ok: false, error: 'blocked' },
  },
  {
    name: 'notifications.getPushSubscriptions',
    call: () => notifications.getPushSubscriptions(),
    method: 'get',
    route: '/notifications/push-subscriptions',
    ok: { subscriptions: [{ id: 1 }] },
    result: [{ id: 1 }],
  },
  {
    name: 'notifications.getPushSubscriptions empty fallback',
    call: () => notifications.getPushSubscriptions(),
    method: 'get',
    route: '/notifications/push-subscriptions',
    result: [],
  },
  {
    name: 'notifications.createPushSubscription',
    call: () => notifications.createPushSubscription({ endpoint: 'e', p256dh: 'p', auth: 'a' }),
    method: 'post',
    route: '/notifications/push-subscriptions',
    body: { endpoint: 'e', p256dh: 'p', auth: 'a' },
    ok: { id: 3 },
    result: { id: 3 },
  },
  {
    name: 'notifications.deletePushSubscription',
    call: () => notifications.deletePushSubscription(3),
    method: 'delete',
    route: '/notifications/push-subscriptions/3',
    ok: NO_CONTENT(),
  },
  {
    name: 'notifications.getDeviceRegistrations',
    call: () => notifications.getDeviceRegistrations(),
    method: 'get',
    route: '/notifications/devices',
    ok: { devices: [{ id: 1 }] },
    result: [{ id: 1 }],
  },
  {
    name: 'notifications.getDeviceRegistrations empty fallback',
    call: () => notifications.getDeviceRegistrations(),
    method: 'get',
    route: '/notifications/devices',
    result: [],
  },
  {
    name: 'notifications.deleteDeviceRegistration',
    call: () => notifications.deleteDeviceRegistration(5),
    method: 'delete',
    route: '/notifications/devices/5',
    ok: NO_CONTENT(),
  },

  // --- occasion events
  {
    name: 'occasionEvents.getOccasionEvents no params',
    call: () => occasionEvents.getOccasionEvents(),
    method: 'get',
    route: '/occasion-events',
    url: '/api/v1/occasion-events?limit=100',
  },
  {
    name: 'occasionEvents.getOccasionEvents all params',
    call: () => occasionEvents.getOccasionEvents({ from: 'f', to: 't', cursor: 'c', limit: 5 }),
    method: 'get',
    route: '/occasion-events',
    url: '/api/v1/occasion-events?limit=5&from=f&to=t&cursor=c',
  },
  {
    name: 'occasionEvents.getOccasionEvent',
    call: () => occasionEvents.getOccasionEvent('e1'),
    method: 'get',
    route: '/occasion-events/e1',
    ok: { attendees: [] },
    result: { attendees: [] },
  },
  {
    name: 'occasionEvents.createOccasionEvent unwraps',
    call: () => occasionEvents.createOccasionEvent({ title: 'T' } as never),
    method: 'post',
    route: '/occasion-events',
    body: { title: 'T' },
    ok: { occasion_event: { id: 'e' } },
    result: { id: 'e' },
  },
  {
    name: 'occasionEvents.updateOccasionEvent raw',
    call: () => occasionEvents.updateOccasionEvent('e1', { title: 'T' } as never),
    method: 'put',
    route: '/occasion-events/e1',
    body: { title: 'T' },
    ok: { id: 'e1' },
    result: { id: 'e1' },
  },
  {
    name: 'occasionEvents.deleteOccasionEvent',
    call: () => occasionEvents.deleteOccasionEvent('e1'),
    method: 'delete',
    route: '/occasion-events/e1',
    ok: NO_CONTENT(),
  },
  {
    name: 'occasionEvents.addOccasionEventAttendee unwraps attendee',
    call: () => occasionEvents.addOccasionEventAttendee('e1', { entity_id: 'u', rsvp: 'accepted' }),
    method: 'post',
    route: '/occasion-events/e1/attendees',
    body: { entity_id: 'u', rsvp: 'accepted' },
    ok: { attendee: { id: 'a' } },
    result: { id: 'a' },
  },
  {
    name: 'occasionEvents.updateOccasionEventAttendee',
    call: () => occasionEvents.updateOccasionEventAttendee('e1', 'u1', 'declined'),
    method: 'put',
    route: '/occasion-events/e1/attendees/u1',
    body: { rsvp: 'declined' },
    ok: { id: 'a' },
    result: { id: 'a' },
  },
  {
    name: 'occasionEvents.removeOccasionEventAttendee',
    call: () => occasionEvents.removeOccasionEventAttendee('e1', 'u1'),
    method: 'delete',
    route: '/occasion-events/e1/attendees/u1',
    ok: NO_CONTENT(),
  },
  {
    name: 'occasionEvents.getInviteeSuggestions circles only',
    call: () => occasionEvents.getInviteeSuggestions({ circleIds: ['a', 'b'] }),
    method: 'get',
    route: '/occasion-events/invitee-suggestions',
    url: '/api/v1/occasion-events/invitee-suggestions?circle_ids=a%2Cb',
    ok: { suggestions: [{ contact_id: 1 }] },
    result: [{ contact_id: 1 }],
  },
  {
    name: 'occasionEvents.getInviteeSuggestions with event + empty fallback',
    call: () => occasionEvents.getInviteeSuggestions({ circleIds: ['a'], eventId: 'e1' }),
    method: 'get',
    route: '/occasion-events/invitee-suggestions',
    url: '/api/v1/occasion-events/invitee-suggestions?circle_ids=a&event_id=e1',
    result: [],
  },

  // --- preferences
  {
    name: 'prefs.getPreferences no params',
    call: () => prefs.getPreferences(),
    method: 'get',
    route: '/preferences',
    url: '/api/v1/preferences?limit=100',
  },
  {
    name: 'prefs.getPreferences all params',
    call: () => prefs.getPreferences({ entityId: 'e', cursor: 'c', limit: 5 }),
    method: 'get',
    route: '/preferences',
    url: '/api/v1/preferences?limit=5&entity_id=e&cursor=c',
  },
  {
    name: 'prefs.createPreference unwraps',
    call: () => prefs.createPreference({ entity_id: 'e', category: 'food', value: 'v' }),
    method: 'post',
    route: '/preferences',
    body: { entity_id: 'e', category: 'food', value: 'v' },
    ok: { preference: { id: 'p' } },
    result: { id: 'p' },
  },
  {
    name: 'prefs.updatePreference raw',
    call: () =>
      prefs.updatePreference('p1', {
        entity_id: 'e',
        category: 'hobby',
        value: 'v',
        level: 'high',
      }),
    method: 'put',
    route: '/preferences/p1',
    body: { entity_id: 'e', category: 'hobby', value: 'v', level: 'high' },
    ok: { id: 'p1' },
    result: { id: 'p1' },
  },
  {
    name: 'prefs.deletePreference',
    call: () => prefs.deletePreference('p1'),
    method: 'delete',
    route: '/preferences/p1',
    ok: NO_CONTENT(),
  },

  // --- reach out
  {
    name: 'reachOut.getReachOutSuggestions',
    call: () => reachOut.getReachOutSuggestions(),
    method: 'get',
    route: '/reach-out-suggestions',
    ok: { suggestions: [] },
    result: { suggestions: [] },
  },
  {
    name: 'reachOut.dismissReachOutSuggestion',
    call: () => reachOut.dismissReachOutSuggestion('r1'),
    method: 'post',
    route: '/reach-out-suggestions/r1/dismiss',
    ok: NO_CONTENT(),
  },

  // --- search
  {
    name: 'search.searchAll without limit',
    call: () => search.searchAll('a b&c'),
    method: 'get',
    route: '/search',
    url: '/api/v1/search?q=a+b%26c',
    ok: { query: 'a b&c' },
    result: { query: 'a b&c' },
  },
  {
    name: 'search.searchAll limit 0 is sent',
    call: () => search.searchAll('q', 0),
    method: 'get',
    route: '/search',
    url: '/api/v1/search?q=q&limit=0',
  },
  {
    name: 'search.rebuildSearchIndex',
    call: () => search.rebuildSearchIndex(),
    method: 'post',
    route: '/admin/search/rebuild',
    ok: NO_CONTENT(),
  },

  // --- sessions (handleResponse based)
  {
    name: 'sessions.getSessions',
    call: () => sessions.getSessions(),
    method: 'get',
    route: '/sessions',
    ok: { sessions: [{ id: 's' }] },
    result: { sessions: [{ id: 's' }] },
    err: 'plain',
  },
  {
    name: 'sessions.getSessions empty fallback',
    call: () => sessions.getSessions(),
    method: 'get',
    route: '/sessions',
    result: { sessions: [] },
    err: 'plain',
  },
  {
    name: 'sessions.revokeSession encodes id',
    call: () => sessions.revokeSession('a/b'),
    method: 'delete',
    route: '/sessions/:id',
    url: '/api/v1/sessions/a%2Fb',
    ok: NO_CONTENT(),
    err: 'plain',
  },
  {
    name: 'sessions.revokeOtherSessions',
    call: () => sessions.revokeOtherSessions(),
    method: 'delete',
    route: '/sessions',
    ok: { revoked: 2 },
    result: { revoked: 2 },
    err: 'plain',
  },

  // --- source import (generic)
  {
    name: 'sourceImport.getSourceImportStatus',
    call: () => sourceImport.getSourceImportStatus('/import/x', 'a&b'),
    method: 'get',
    route: '/import/x/status',
    url: '/api/v1/import/x/status?session_id=a%26b',
    ok: { phase: 'ready' },
    result: { phase: 'ready' },
  },
  {
    name: 'sourceImport.getSourceImportPreview',
    call: () => sourceImport.getSourceImportPreview('/import/x', 's'),
    method: 'get',
    route: '/import/x/preview',
    url: '/api/v1/import/x/preview?session_id=s',
    ok: { rows: [] },
    result: { rows: [] },
  },
  {
    name: 'sourceImport.confirmSourceImport',
    call: () =>
      sourceImport.confirmSourceImport('/import/x', 's', [{ row_index: 0, action: 'update' }]),
    method: 'post',
    route: '/import/x/confirm',
    body: { session_id: 's', actions: [{ row_index: 0, action: 'update' }] },
    ok: NO_CONTENT(),
  },

  // --- system status
  {
    name: 'systemStatus.getSystemStatus',
    call: () => systemStatus.getSystemStatus(),
    method: 'get',
    route: '/admin/system-status',
    ok: { overall: 'healthy' },
    result: { overall: 'healthy' },
  },

  // --- webhooks (handleResponse based)
  {
    name: 'webhooks.getWebhooks',
    call: () => webhooks.getWebhooks(),
    method: 'get',
    route: '/webhooks',
    ok: { webhooks: [{ id: 1 }] },
    result: [{ id: 1 }],
    err: 'plain',
  },
  {
    name: 'webhooks.getWebhooks empty fallback',
    call: () => webhooks.getWebhooks(),
    method: 'get',
    route: '/webhooks',
    result: [],
    err: 'plain',
  },
  {
    name: 'webhooks.createWebhook',
    call: () =>
      webhooks.createWebhook({ name: 'n', url: 'https://x', events: ['a'], is_active: true }),
    method: 'post',
    route: '/webhooks',
    body: { name: 'n', url: 'https://x', events: ['a'], is_active: true },
    ok: { id: 1, secret: 's' },
    result: { id: 1, secret: 's' },
    err: 'plain',
  },
  {
    name: 'webhooks.updateWebhook',
    call: () =>
      webhooks.updateWebhook(1, { name: 'n', url: 'https://x', events: [], is_active: false }),
    method: 'put',
    route: '/webhooks/1',
    body: { name: 'n', url: 'https://x', events: [], is_active: false },
    ok: { id: 1 },
    result: { id: 1 },
    err: 'plain',
  },
  {
    name: 'webhooks.deleteWebhook',
    call: () => webhooks.deleteWebhook(1),
    method: 'delete',
    route: '/webhooks/1',
    ok: NO_CONTENT(),
    err: 'plain',
  },
  {
    name: 'webhooks.testWebhook',
    call: () => webhooks.testWebhook(1),
    method: 'post',
    route: '/webhooks/1/test',
    ok: { delivery: { id: 9 } },
    result: { delivery: { id: 9 } },
    err: 'plain',
  },
  {
    name: 'webhooks.getWebhookDeliveries',
    call: () => webhooks.getWebhookDeliveries(1),
    method: 'get',
    route: '/webhooks/1/deliveries',
    ok: { deliveries: [{ id: 1 }] },
    result: [{ id: 1 }],
    err: 'plain',
  },
  {
    name: 'webhooks.getWebhookDeliveries empty fallback',
    call: () => webhooks.getWebhookDeliveries(1),
    method: 'get',
    route: '/webhooks/1/deliveries',
    result: [],
    err: 'plain',
  },
];

function register(row: Row, response: JsonBodyType | Response): Captured[] {
  // A `:id` route segment (sessions) needs msw's own path param syntax; mockApi
  // already passes the route straight through.
  return mockApi(row.method, row.route, response);
}

describe('request contracts', () => {
  test.each(rows)(
    '$name sends the documented request and returns the parsed result',
    async (row) => {
      const calls = register(row, row.ok ?? {});
      const result = await row.call();
      expect(calls).toHaveLength(1);
      const c = calls[0];
      expect(c.method).toBe(row.method.toUpperCase());
      expect(c.url).toBe(row.url ?? `/api/v1${row.route}`);
      if (row.body === undefined) {
        expect(c.body).toBeUndefined();
      } else {
        expect(c.body).toEqual(row.body);
        expect(c.headers.get('content-type')).toBe('application/json');
      }
      if (row.result !== undefined) expect(result).toEqual(row.result);
    },
  );
});

describe('error mapping', () => {
  test.each(rows)('$name maps a 403 envelope to its typed error', async (row) => {
    register(row, errorEnvelope(403, 'FORBIDDEN', 'nope', undefined, 'req-9'));
    const err = await row.call().then(
      () => {
        throw new Error('expected rejection');
      },
      (e: unknown) => e,
    );
    expect(err).toBeInstanceOf(Error);
    expect((err as Error).message).toBe('nope');
    if ((row.err ?? 'api') === 'api') {
      expect(err).toBeInstanceOf(ApiError);
      const apiErr = err as ApiError;
      expect(apiErr.status).toBe(403);
      expect(apiErr.code).toBe('FORBIDDEN');
      expect(apiErr.requestId).toBe('req-9');
    } else {
      expect(err).not.toBeInstanceOf(ApiError);
    }
  });
});

describe('source import: cancelSourceImport is best-effort', () => {
  test('POSTs cancel with an encoded session id', async () => {
    const calls = mockApi('post', '/import/x/cancel', NO_CONTENT());
    await sourceImport.cancelSourceImport('/import/x', 'a&b');
    expect(calls[0].method).toBe('POST');
    expect(calls[0].url).toBe('/api/v1/import/x/cancel?session_id=a%26b');
  });

  test('swallows a network failure', async () => {
    server.use(http.post(`${API_BASE_URL}/import/x/cancel`, () => HttpResponse.error()));
    await expect(sourceImport.cancelSourceImport('/import/x', 's')).resolves.toBeUndefined();
  });

  test('the wrappers target their own base path', async () => {
    const m = mockApi('post', '/contacts/import/meerkat/cancel', NO_CONTENT());
    const o = mockApi('post', '/contacts/import/monica/cancel', NO_CONTENT());
    const y = mockApi('post', '/import/mycorrhizal/cancel', NO_CONTENT());
    await meerkat.cancelMeerkatImport('s');
    await monica.cancelMonicaImport('s');
    await mycorrhizal.cancelMycorrhizalImport('s');
    expect(m).toHaveLength(1);
    expect(o).toHaveLength(1);
    expect(y).toHaveLength(1);
  });
});

describe('health', () => {
  test('getHealth hits /health at the server root, not under /api/v1', async () => {
    let seen = '';
    server.use(
      http.get(/\/health$/, ({ request }) => {
        seen = new URL(request.url).pathname;
        return HttpResponse.json({ status: 'healthy', version: '1.0.0' });
      }),
    );
    expect(await health.getHealth()).toEqual({ status: 'healthy', version: '1.0.0' });
    expect(seen).toBe('/health');
  });

  test('getHealth maps an error envelope', async () => {
    server.use(http.get(/\/health$/, () => errorEnvelope(503, 'UNAVAILABLE', 'db down')));
    const err = await health.getHealth().catch((e) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err.status).toBe(503);
  });

  test('formatBuildVersion with and without commit', () => {
    expect(health.formatBuildVersion({ version: 'v1', commit: 'abc' })).toBe('v1 (abc)');
    expect(health.formatBuildVersion({ version: 'v1' })).toBe('v1');
  });
});

describe('auth.deleteOwnAccount request body', () => {
  test('sends only the password by default, then totp and promote_user_id when given', async () => {
    const calls = mockApi('delete', '/account', { message: 'bye' });
    expect(await auth.deleteOwnAccount('pw')).toBe('bye');
    await auth.deleteOwnAccount('pw', '123456', 0);
    expect(calls[0].method).toBe('DELETE');
    expect(calls[0].body).toEqual({ current_password: 'pw' });
    expect(calls[1].body).toEqual({
      current_password: 'pw',
      totp_code: '123456',
      promote_user_id: 0,
    });
  });
});
