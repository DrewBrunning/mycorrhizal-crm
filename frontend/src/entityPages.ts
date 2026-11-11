// One lazy-load target for both entity pages (#2448). Importing EntitiesPage and
// EntityDetailPage as two separate React.lazy() chunks made the bundler hoist the
// app's shared core (all five locales, i18n config, API client) into a 288 KiB
// "errorHandler" chunk; a single dynamic-import target keeps that core in `index`.
export { default as EntitiesPage } from './EntitiesPage';
export { default as EntityDetailPage } from './EntityDetailPage';
