// COMPAT-02 (issue #473). Loaded only via <script nomodule> in index.html --
// a browser new enough to support ES modules never requests this file at
// all, so it never runs there. Deliberately plain ES5 (var, string
// concatenation, no arrow functions, no template literals, no let/const)
// because a browser old enough to need this cannot parse anything newer, and
// deliberately not wired into i18next: this has to run before the framework
// (and i18next) can load. See docs/development/supported-runtime-matrix.md
// for the documented floor this replaces a blank page for.
// biome-ignore lint/complexity/useArrowFunction: ES6 -- never auto-fix this file.
(function () {
  var root = document.getElementById('root');
  if (root) {
    root.innerHTML =
      '<div style="font-family: system-ui, sans-serif; max-width: 32em; margin: 3em auto; padding: 0 1em; line-height: 1.5;">' +
      '<h1 style="font-size: 1.25em;">Please update your browser</h1>' +
      '<p>Mycorrhizal CRM needs a browser released in 2023 or later ' +
      '(Chrome, Edge or Firefox 111 or later, Safari 16.4 or later). ' +
      'This browser is too old to run it.</p>' +
      '</div>';
  }
})();
