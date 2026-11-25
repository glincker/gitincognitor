package cli

// version is set at release time with -ldflags "-X ...internal/cli.version=vX.Y.Z".
var version = "dev"

func init() { commands["version"] = cmdVersion }

func cmdVersion(c *Ctx, _ []string) error {
	c.Emit(map[string]string{"version": version}, version)
	return nil
}
