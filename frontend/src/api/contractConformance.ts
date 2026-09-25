// Compile-time frontend↔backend contract conformance.
//
// The web client's API types in this directory are hand-written. The
// backend is validated against backend/openapi.yaml (TestOpenAPIRouteCoverage,
// TestOpenAPIResponseSpotCheck, Schemathesis), and ../generated/openapi.ts is
// generated from that same spec (`cd backend && go run ./cmd/gentsapi`). This
// file pins the hand-written types to the generated ones, so `tsc --noEmit`
// fails when they drift. It is types-only and imported by nothing — it adds
// zero bytes to the production bundle.
//
// What a response check (`NoDrift<ResponseDrift<Hand, Spec>>`) enforces:
//   - unknownFields: every field the TS type declares exists in the spec (a
//     renamed or invented field reads as `undefined` forever).
//   - typeMismatch: for every shared field, the spec's value type is
//     assignable to the TS field type — enum values, nullability (`| null`)
//     and nested object shapes included.
//   - requiredButOptionalInSpec: a field the TS type marks required must be
//     `required:` in the spec. This is CLAUDE.md frontend trap #8 (a Go
//     `omitempty` field typed as required in TS). Fix by making the TS field
//     optional, or — only when the Go struct really always emits it (no
//     `omitempty`) — by adding it to the schema's `required:` list.
//   The TS type may OMIT spec fields it does not use; that is not drift.
//
// Enum lists (trap #4, hand-mirrored `oneof` validators) use `SameMembers`,
// which is exact in both directions.
//
// A mismatch shows up as a tsc error on the offending line naming the keys,
// e.g. `Type '"ID"' does not satisfy the constraint 'never'`. Deliberate
// differences are written as an explicit `Omit<>`/`DeliberateException`
// with a comment saying why.
import type * as S from '../generated/openapi';
import type { Activity, ActivityContact } from './activities';
import type { BriefingRelationship } from './briefings';
import type { CadenceHealth, CadencePolicy, OverdueCadence } from './cadencePolicies';
import type { Circle, CircleMember } from './circles';
import type {
  ContactDetailImmich,
  ContactDetailLifeEvent,
  ContactDetailResponse,
  ContactDetailUser,
  ImmichPersonSummary,
} from './contactDetail';
import type { ContactSyncConflict } from './contactSyncConflicts';
import type {
  Birthday,
  Card,
  CardAddress,
  CardAnniversary,
  CardName,
  ContactRecordResponse,
  ContactSummaryDTO,
  CRMEnvelope,
} from './contacts';
import type { ConversationAgenda } from './conversationAgenda';
import type { DashboardReminder, DashboardResponse } from './dashboard';
import type { ExternalActivity, ExternalIdentity } from './externalLinks';
import type { FieldDefinition, FieldValue } from './fieldDefinitions';
import type { GIFT_STATUSES, Gift } from './gifts';
import type { HOUSEHOLD_TYPES, Household, HouseholdMember } from './households';
import type { LIFE_EVENT_CATEGORIES, LifeEvent, LifeEventSuggestion } from './lifeEvents';
import type { LinkFieldType } from './linkFieldTypes';
import type { Note } from './notes';
import type {
  InviteeSuggestion,
  OccasionEvent,
  OccasionEventAttendee,
  OccasionEventAttendeeView,
  OccasionEventRSVP,
} from './occasionEvents';
import type {
  GiftShoppingItem,
  OccasionObligation,
  OccasionSensitivity,
  UpcomingOccasion,
} from './occasionObligations';
import type { Preference, PreferenceLevel, PreferenceSource } from './preferences';
import type { ReachOutSuggestion } from './reachOutSuggestions';
import type {
  RelationshipEdge,
  RelationshipEdgeSensitivity,
  RelationshipEdgeSource,
  RelationshipEdgeStatus,
} from './relationshipEdges';
import type { Reminder, ReminderCompletion } from './reminders';
import type { ContactTag, Tag } from './tags';
import type { TIMELINE_TYPES } from './timeline';

// ---------------------------------------------------------------- helpers

type Defined<T> = Exclude<T, undefined>;

// Keys of T that are not optional.
type RequiredKeys<T> = {
  [K in keyof T]-?: object extends Pick<T, K> ? never : K;
}[keyof T];

type MismatchedKeys<Hand, Spec> = {
  [K in keyof Hand & keyof Spec]-?: [Defined<Spec[K]>] extends [Defined<Hand[K]>] ? never : K;
}[keyof Hand & keyof Spec];

/** The three drift classes a hand-written response type can have vs the spec. */
export type ResponseDrift<Hand, Spec> = {
  unknownFields: Exclude<keyof Hand, keyof Spec>;
  typeMismatch: MismatchedKeys<Hand, Spec>;
  requiredButOptionalInSpec: Exclude<RequiredKeys<Hand>, RequiredKeys<Spec>>;
};

/** Compiles only when every drift class is `never`. */
export type NoDrift<
  T extends { unknownFields: never; typeMismatch: never; requiredButOptionalInSpec: never },
> = T;

/**
 * A documented, deliberate difference: the listed keys are excluded from the
 * check. Every use must appear in the register below with its reason.
 *
 * Register:
 *  - LifeEvent.type / ContactDetailLifeEvent.type — Go `json:"type,omitempty"`
 *    and unvalidated, so the spec correctly calls it optional; the TS type
 *    keeps it required because ~8 components build `lifeEvent.types.${type}`
 *    i18n keys from it. Known trap-#8 exposure (an event with an empty type
 *    would render the raw key), left as a follow-up rather than a component
 *    refactor in the contract-check change.
 *  - DashboardResponse.random_contacts / favorites — typed as the flat list
 *    `Contact`, which is a different (post-adapter) shape from the wire's
 *    ContactResponse (legacy gorm serialization: PascalCase ID, collection
 *    fields `null` when empty, no `kind`/`uid`). The dashboard reads only
 *    name/photo/ID-ish fields from these rows.
 *  - ContactSyncConflict.field — the TS type narrows the Go free-form string
 *    to the known flat-field tokens for display; the spec has no enum for it.
 *  - DashboardResponse.contact_sync_conflicts / ContactDetailResponse.life_events
 *    — excepted only because their element types carry one of the exceptions
 *    above; the element types themselves are checked on their own lines.
 */
export type DeliberateException<T, K extends keyof T> = Omit<T, K>;

/** Compiles only when A and B have exactly the same members. */
export type SameMembers<A, B> = [A] extends [B] ? ([B] extends [A] ? true : false) : false;
export type Assert<T extends true> = T;

// ---------------------------------------------------------------- entities

export type Conformance = [
  NoDrift<ResponseDrift<Note, S.Note>>,
  NoDrift<ResponseDrift<Activity, S.Activity>>,
  NoDrift<ResponseDrift<Reminder, S.Reminder>>,
  NoDrift<ResponseDrift<ReminderCompletion, S.ReminderCompletion>>,
  NoDrift<ResponseDrift<RelationshipEdge, S.RelationshipEdge>>,
  NoDrift<ResponseDrift<DeliberateException<LifeEvent, 'type'>, S.LifeEvent>>,
  NoDrift<ResponseDrift<LifeEventSuggestion, S.LifeEventSuggestion>>,
  NoDrift<ResponseDrift<Circle, S.Circle>>,
  NoDrift<ResponseDrift<CircleMember, S.CircleMember>>,
  NoDrift<ResponseDrift<Tag, S.Tag>>,
  NoDrift<ResponseDrift<ContactTag, S.ContactTag>>,
  NoDrift<ResponseDrift<Household, S.Household>>,
  NoDrift<ResponseDrift<HouseholdMember, S.HouseholdMember>>,
  NoDrift<ResponseDrift<Gift, S.Gift>>,
  NoDrift<ResponseDrift<Preference, S.Preference>>,
  NoDrift<ResponseDrift<OccasionObligation, S.OccasionObligation>>,
  NoDrift<ResponseDrift<UpcomingOccasion, S.UpcomingOccasion>>,
  NoDrift<ResponseDrift<GiftShoppingItem, S.GiftShoppingItem>>,
  NoDrift<ResponseDrift<OccasionEvent, S.OccasionEvent>>,
  NoDrift<ResponseDrift<OccasionEventAttendee, S.OccasionEventAttendee>>,
  NoDrift<ResponseDrift<OccasionEventAttendeeView, S.OccasionEventAttendeeView>>,
  NoDrift<ResponseDrift<InviteeSuggestion, S.InviteeSuggestion>>,
  NoDrift<ResponseDrift<ConversationAgenda, S.ConversationAgenda>>,
  NoDrift<ResponseDrift<CadencePolicy, S.CadencePolicyWithHealth>>,
  NoDrift<ResponseDrift<CadenceHealth, S.CadenceHealth>>,
  NoDrift<ResponseDrift<OverdueCadence, S.OverdueCadence>>,
  NoDrift<ResponseDrift<ReachOutSuggestion, S.ReachOutSuggestion>>,
  NoDrift<ResponseDrift<ExternalIdentity, S.ExternalIdentity>>,
  NoDrift<ResponseDrift<ExternalActivity, S.ExternalActivity>>,
  NoDrift<ResponseDrift<FieldDefinition, S.FieldDefinition>>,
  NoDrift<ResponseDrift<FieldValue, S.FieldValue>>,
  NoDrift<ResponseDrift<LinkFieldType, S.LinkFieldType>>,
  NoDrift<ResponseDrift<Birthday, S.Birthday>>,
  NoDrift<ResponseDrift<DashboardReminder, S.DashboardReminder>>,
  NoDrift<
    ResponseDrift<
      DeliberateException<
        DashboardResponse,
        'random_contacts' | 'favorites' | 'contact_sync_conflicts'
      >,
      S.DashboardResponse
    >
  >,
  NoDrift<ResponseDrift<ContactSummaryDTO, S.ContactSummary>>,
  NoDrift<ResponseDrift<ContactRecordResponse, S.ContactRecordResponse>>,
  NoDrift<ResponseDrift<CRMEnvelope, S.CRMEnvelope>>,
  NoDrift<ResponseDrift<Card, S.Card>>,
  NoDrift<ResponseDrift<ContactDetailUser, S.ContactDetailUser>>,
  NoDrift<
    ResponseDrift<
      DeliberateException<ContactDetailResponse, 'life_events'>,
      S.ContactDetailResponse
    >
  >,
  NoDrift<ResponseDrift<ActivityContact, S.ContactFlat>>,
  NoDrift<ResponseDrift<DeliberateException<ContactSyncConflict, 'field'>, S.ContactSyncConflict>>,
  NoDrift<ResponseDrift<BriefingRelationship, S.BriefingRelationship>>,
  NoDrift<
    ResponseDrift<DeliberateException<ContactDetailLifeEvent, 'type'>, S.ContactDetailLifeEvent>
  >,
  NoDrift<ResponseDrift<ContactDetailImmich, S.ContactDetailImmich>>,
  NoDrift<ResponseDrift<ImmichPersonSummary, S.ImmichPersonSummary>>,
  NoDrift<ResponseDrift<CardName, S.Name>>,
  NoDrift<ResponseDrift<CardAddress, S.Address>>,
  NoDrift<ResponseDrift<CardAnniversary, S.Anniversary>>,
];

// ---------------------------------------------------------------- enums

export type EnumConformance = [
  Assert<SameMembers<(typeof GIFT_STATUSES)[number], Defined<S.Gift['status']>>>,
  Assert<SameMembers<(typeof LIFE_EVENT_CATEGORIES)[number], Defined<S.LifeEvent['category']>>>,
  Assert<SameMembers<(typeof HOUSEHOLD_TYPES)[number], Defined<S.Household['type']>>>,
  Assert<SameMembers<(typeof TIMELINE_TYPES)[number], S.TimelineItem['type']>>,
  Assert<SameMembers<OccasionEventRSVP, Defined<S.OccasionEventAttendee['rsvp']>>>,
  Assert<SameMembers<OccasionSensitivity, Defined<S.OccasionObligation['sensitivity']>>>,
  Assert<SameMembers<PreferenceLevel, Defined<S.Preference['level']>>>,
  Assert<SameMembers<PreferenceSource, Defined<S.Preference['source']>>>,
  Assert<SameMembers<RelationshipEdgeSource, Defined<S.RelationshipEdge['source']>>>,
  Assert<SameMembers<RelationshipEdgeStatus, Defined<S.RelationshipEdge['status']>>>,
  Assert<SameMembers<RelationshipEdgeSensitivity, Defined<S.RelationshipEdge['sensitivity']>>>,
  Assert<SameMembers<Reminder['recurrence'], Defined<S.Reminder['recurrence']>>>,
];
