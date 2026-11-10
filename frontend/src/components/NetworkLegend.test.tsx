import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, expect, test } from 'vitest';
import '../i18n/config';
import NetworkLegend from './NetworkLegend';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(cleanup);

test('renders only the contact legend item by default', () => {
  render(<NetworkLegend />);

  expect(screen.getByText('Contact')).toBeInTheDocument();
  expect(screen.queryByText('Relationship')).not.toBeInTheDocument();
  expect(screen.queryByText('Shared Activity')).not.toBeInTheDocument();
  expect(screen.queryByText('Circle')).not.toBeInTheDocument();
});

test('showRelationships adds the relationship line item', () => {
  render(<NetworkLegend showRelationships />);

  expect(screen.getByText('Relationship')).toBeInTheDocument();
  expect(screen.queryByText('Shared Activity')).not.toBeInTheDocument();
});

test('showActivities adds the shared activity and activity items', () => {
  render(<NetworkLegend showActivities />);

  expect(screen.getByText('Shared Activity')).toBeInTheDocument();
  expect(screen.getByText('Activity')).toBeInTheDocument();
  expect(screen.queryByText('Circle')).not.toBeInTheDocument();
});

test('showCircles adds the circle membership and circle items', () => {
  render(<NetworkLegend showCircles />);

  expect(screen.getByText('Circle Membership')).toBeInTheDocument();
  expect(screen.getByText('Circle')).toBeInTheDocument();
  expect(screen.queryByText('Relationship')).not.toBeInTheDocument();
});

test('renders every legend item when all toggles are on', () => {
  render(<NetworkLegend showRelationships showActivities showCircles />);

  expect(screen.getByText('Relationship')).toBeInTheDocument();
  expect(screen.getByText('Shared Activity')).toBeInTheDocument();
  expect(screen.getByText('Activity')).toBeInTheDocument();
  expect(screen.getByText('Circle Membership')).toBeInTheDocument();
  expect(screen.getByText('Circle')).toBeInTheDocument();
  expect(screen.getByText('Contact')).toBeInTheDocument();
});

test('showEntities adds the entity node and "linked to" edge items', () => {
  render(<NetworkLegend showEntities />);
  expect(screen.getByText('Organisation, place or pet')).toBeInTheDocument();
  expect(screen.getByText('Linked to')).toBeInTheDocument();
});

test('the entity items are absent by default', () => {
  render(<NetworkLegend />);
  expect(screen.queryByText('Organisation, place or pet')).not.toBeInTheDocument();
});
