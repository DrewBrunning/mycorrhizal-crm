package a

import apperrors "mycorrhizal/errors"

func inTest() { _ = apperrors.ErrConflict("skipped in tests") }
