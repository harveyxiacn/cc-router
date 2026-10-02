package cli

import (
	"encoding/json"
	"errors"
)

func (a *App) backup(args []string) (int, error) {
	encode := func(value any, err error) (int, error) {
		if err != nil {
			return 0, err
		}
		return 0, json.NewEncoder(a.Out).Encode(value)
	}
	if len(args) == 1 {
		switch args[0] {
		case "create":
			info, err := a.Store.CreateBackup()
			return encode(info, err)
		case "list":
			list, err := a.Store.ListBackups()
			return encode(list, err)
		}
	}
	if len(args) == 2 && args[0] == "show" {
		preview, err := a.Store.PreviewBackup(args[1])
		return encode(preview, err)
	}
	if len(args) == 4 && args[0] == "restore" && args[2] == "--reviewed-digest" {
		info, err := a.Store.RestoreBackup(args[1], args[3])
		return encode(info, err)
	}
	return 0, errors.New("usage: cc-router backup create|list|show ID|restore ID --reviewed-digest SHA256; first review backup show output")
}
