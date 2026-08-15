import tseslint from 'typescript-eslint';
import reactHooks from 'eslint-plugin-react-hooks';
import react from 'eslint-plugin-react';
import security from 'eslint-plugin-security';

export default tseslint.config(
  {
    ignores: [
      'build/**',
      'dist/**',
      'node_modules/**',
      'playwright-report/**',
      'test-results/**',
      'coverage/**',
    ],
  },
  tseslint.configs.recommended,
  {
    files: ['**/*.{ts,tsx}'],
    plugins: {
      'react-hooks': reactHooks,
      react,
      security,
    },
    rules: {
      'react-hooks/rules-of-hooks': 'error',
      'react-hooks/exhaustive-deps': 'warn',
      // Browser XSS / tabnabbing guards.
      'react/no-danger': 'error',
      'react/jsx-no-target-blank': 'error',
      'security/detect-eval-with-expression': 'error',
      'security/detect-non-literal-regexp': 'error',
      'security/detect-unsafe-regex': 'error',
      // The api/ layer parses responses as Record<string, any>; tightening it
      // is a separate refactor. Keep `any` visible (warning) but non-blocking.
      '@typescript-eslint/no-explicit-any': 'warn',
      // `_`-prefixed bindings are deliberately ignored (destructure-and-ignore).
      '@typescript-eslint/no-unused-vars': [
        'error',
        { argsIgnorePattern: '^_', varsIgnorePattern: '^_' },
      ],
    },
  },
  {
    // Playwright specs legitimately build RegExps from variables in assertions.
    files: ['e2e/**'],
    rules: {
      'security/detect-non-literal-regexp': 'off',
    },
  },
);
