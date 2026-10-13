import { useRef } from 'react';

// useRowKeys hands out stable React keys for a controlled list, so structural edits
// (removing/inserting a row in the middle) don't re-key the sibling rows below and
// remount their inputs — which would otherwise drop focus mid-edit.
//
// Call onRemove(index)/onAdd() right next to the matching value mutation. The
// reconcile against `length` on each render covers external length changes (e.g. a
// parent resetting the list) by padding/truncating keys from the end.
export function useRowKeys(length: number) {
  const keys = useRef<number[]>([]);
  const next = useRef(0);

  while (keys.current.length < length) {
    keys.current.push(next.current++);
  }
  // Stryker disable next-line ConditionalExpression,EqualityOperator: the padding loop above leaves keys.length >= length, so truncating at equality (or always) is a no-op
  if (keys.current.length > length) {
    keys.current.length = length;
  }

  return {
    keyAt: (index: number) => keys.current[index],
    onRemove: (index: number) => {
      keys.current.splice(index, 1);
    },
    // Stryker disable next-line BlockStatement: the reconcile loop pads a missing key with the same next id, so an explicit push is redundant when the length grows with it
    onAdd: () => {
      keys.current.push(next.current++);
    },
  };
}
