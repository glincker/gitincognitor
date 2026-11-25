//go:build !darwin && !linux

package cli

// No portable O_NOFOLLOW here, the Lstat and SameFile checks still apply.
const openNoFollow = 0
