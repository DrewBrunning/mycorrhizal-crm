Suggested relationship edges are appearing in the relationship graph and
influencing traversal. Only confirmed edges should be treated as fact; suggested
edges are review material and must never be projected, graphed, or followed as a
hop.

Fix the query/projection path that lets them through, and add regression tests
covering both the graph response and the neutral-card projection. Check that
inverse derivation and the browser graph budget still behave.
