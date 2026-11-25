// local/no-raw-error-message (issue #2920 item 1, follow-up to #1915).
//
// A thrown Error's `.message` is English (a browser's "Failed to fetch", a
// WebAuthn DOMException, an ApiError's server-side sentence). Shown in the UI
// it reaches every language untranslated, so UI code goes through
// getErrorMessage() (src/utils/errorHandler.ts), which maps network failures
// and ApiError reason codes to translated text.
//
// The first version of this guard was a `no-restricted-syntax` selector for
// one spelling, `err instanceof Error ? err.message : x`. The same defect also
// shipped as `err instanceof Error && err.message ? err.message : x`, as an
// `if (err instanceof Error)` block, and as a typed `(error: Error) =>
// error.message` callback. This rule is type-aware instead: it reports any
// `.message` read from an Error-typed value that is used as a *value*, and
// leaves alone the reads that only inspect the message (a comparison, a method
// call on it, a console/log argument, or wrapping it in a new Error).

/** True when `type` is, or extends, Error/DOMException (unions: any member). */
function isErrorLike(type, checker, seen = new Set()) {
  if (!type || seen.has(type)) return false;
  seen.add(type);
  if (type.isUnion()) return type.types.some((t) => isErrorLike(t, checker, seen));
  const name = type.getSymbol()?.getName();
  if (name === 'Error' || name === 'DOMException') return true;
  if (type.isClassOrInterface()) {
    return checker.getBaseTypes(type).some((base) => isErrorLike(base, checker, seen));
  }
  const apparent = checker.getApparentType(type);
  if (apparent !== type) return isErrorLike(apparent, checker, seen);
  return false;
}

const COMPARISON = new Set(['===', '!==', '==', '!=', 'in', 'instanceof']);

/** Unwraps `a?.b` chains and TS-only wrappers so the real parent is checked. */
function valueParent(node) {
  let child = node;
  let parent = node.parent;
  while (
    parent &&
    (parent.type === 'ChainExpression' ||
      parent.type === 'TSNonNullExpression' ||
      parent.type === 'TSAsExpression')
  ) {
    child = parent;
    parent = parent.parent;
  }
  return { child, parent };
}

function isConsoleOrLogCall(callee) {
  if (callee.type === 'MemberExpression' && callee.object.type === 'Identifier') {
    return callee.object.name === 'console';
  }
  return (
    callee.type === 'Identifier' && /^(logError|reportError|captureException)$/.test(callee.name)
  );
}

/** The read only inspects the message; it never becomes UI text. */
function isInspection(node) {
  const { child, parent } = valueParent(node);
  if (!parent) return false;
  if (parent.type === 'BinaryExpression' && COMPARISON.has(parent.operator)) return true;
  // `err.message.includes('x')`, `err.message.length`: a read *of* the message.
  if (parent.type === 'MemberExpression' && parent.object === child) return true;
  // `new Error(err.message)` re-wraps; it is not displayed here.
  if (parent.type === 'NewExpression') return true;
  if (parent.type === 'CallExpression' && parent.arguments.includes(child)) {
    if (isConsoleOrLogCall(parent.callee)) return true;
    // `SET.has(err.message)`, `re.test(err.message)`: a lookup, not display.
    if (
      parent.callee.type === 'MemberExpression' &&
      parent.callee.property.type === 'Identifier' &&
      /^(has|includes|test|match|startsWith|endsWith|indexOf)$/.test(parent.callee.property.name)
    ) {
      return true;
    }
  }
  // `if (err.message)` / `err.message ? a : b` / `!err.message`: a truthiness test.
  if (parent.type === 'IfStatement' && parent.test === child) return true;
  if (parent.type === 'ConditionalExpression' && parent.test === child) return true;
  if (parent.type === 'UnaryExpression' && parent.operator === '!') return true;
  if (parent.type === 'LogicalExpression' && parent.operator === '&&' && parent.left === child) {
    return true;
  }
  if (
    parent.type === 'LogicalExpression' &&
    parent.operator === '&&' &&
    parent.right === child &&
    isTestPosition(parent)
  ) {
    return true;
  }
  return false;
}

/** True when the expression only feeds a condition (`a && err.message ? x : y`). */
function isTestPosition(expr) {
  const { child, parent } = valueParent(expr);
  if (!parent) return false;
  if (parent.type === 'ConditionalExpression' && parent.test === child) return true;
  if (parent.type === 'IfStatement' && parent.test === child) return true;
  if (parent.type === 'UnaryExpression' && parent.operator === '!') return true;
  if (parent.type === 'LogicalExpression' && parent.operator === '&&')
    return isTestPosition(parent);
  return false;
}

export default {
  meta: {
    type: 'problem',
    docs: {
      description: 'Disallow showing a raw Error.message; use getErrorMessage() (#1915, #2920).',
    },
    messages: {
      raw: 'Use getErrorMessage(err) from utils/errorHandler (#1915): a raw Error.message is untranslated English.',
    },
    schema: [],
  },
  create(context) {
    const services = context.sourceCode.parserServices;
    if (!services?.program || !services.esTreeNodeToTSNodeMap) {
      throw new Error('local/no-raw-error-message needs type information (projectService).');
    }
    const checker = services.program.getTypeChecker();
    return {
      MemberExpression(node) {
        if (node.computed || node.property.type !== 'Identifier') return;
        if (node.property.name !== 'message') return;
        if (isInspection(node)) return;
        const tsNode = services.esTreeNodeToTSNodeMap.get(node.object);
        if (!isErrorLike(checker.getTypeAtLocation(tsNode), checker)) return;
        context.report({ node, messageId: 'raw' });
      },
    };
  },
};
