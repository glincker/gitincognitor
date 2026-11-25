//go:build darwin || linux

package cli

import "syscall"

// openNoFollow makes open fail on a symlink swapped in after the plan.
const openNoFollow = syscall.O_NOFOLLOW
