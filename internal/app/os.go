package app

import (
	"os"

	"meofishes/internal/catalog"
)

var (
	osStat      = os.Stat
	osRemoveAll = os.RemoveAll
)

type catalogEntry = catalog.Entry
