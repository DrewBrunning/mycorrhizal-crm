/// <reference types="vite/client" />

// vite/client provides ambient types for static asset imports (images, SVGs,
// CSS Modules, `import.meta.env`) but not for plain side-effect stylesheet
// imports (e.g. `import './App.css'`). Under `moduleResolution: "bundler"`
// TypeScript requires a declaration for these, otherwise it reports TS2882.
declare module '*.css';
declare module '*.scss';
declare module '*.sass';
