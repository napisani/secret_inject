package main

import (
	"log/slog"
  "fmt"
)

func ExportAsEnvVars(secrets *Secrets) {
	slog.Debug("Exporting secrets as environment variables")
  str := ""
  
	for key, value := range secrets.Entries {
    str += fmt.Sprintf("export %s='%s'\n", key, value)
	}
  fmt.Println(str)
}
