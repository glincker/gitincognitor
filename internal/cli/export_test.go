package cli

// Test-only setters for the adopt seams.
func SetAdoptBeforeCopy(f func()) { adoptBeforeCopy = f }
func SetAdoptAfterSync(f func())  { adoptAfterSync = f }
