package constants_test

import (
	"testing"

	"github.com/libost/sticker_go/constants"
)

func TestSetBaseDir(t *testing.T) {
	bases := []string{
		"",
		"./",
		"/etc/sticker_go",
	}
	for _, base := range bases {
		constants.SetBaseDir(base)
		wantCacheDir := base + "/cache/"
		wantConfigFile := base + "/config.yaml"
		wantLogDir := base + "/logs/"
		wantDatabaseFile := base + "/sticker_go.db"
		if base == "" || base == "./" {
			wantCacheDir = "./cache/"
			wantConfigFile = "./config.yaml"
			wantLogDir = "./logs/"
			wantDatabaseFile = "./sticker_go.db"
		}
		if constants.CacheDir != wantCacheDir {
			t.Errorf("CacheDir = %s, want %s", constants.CacheDir, wantCacheDir)
		}
		if constants.ConfigFile != wantConfigFile {
			t.Errorf("ConfigFile = %s, want %s", constants.ConfigFile, wantConfigFile)
		}
		if constants.LogDir != wantLogDir {
			t.Errorf("LogDir = %s, want %s", constants.LogDir, wantLogDir)
		}
		if constants.DatabaseFile != wantDatabaseFile {
			t.Errorf("DatabaseFile = %s, want %s", constants.DatabaseFile, wantDatabaseFile)
		}
	}
}
