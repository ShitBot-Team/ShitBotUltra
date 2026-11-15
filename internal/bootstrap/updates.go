package bootstrap

import (
	"context"
	"path/filepath"

	"github.com/ShitBot-Team/ShitBotUltra/internal/securefs"
	"github.com/ShitBot-Team/ShitBotUltra/internal/updating"
	"github.com/ShitBot-Team/ShitBotUltra/sdk/update"
)

func (e *Engine) ConfigureUpdates(options *update.Options) error {
	if options == nil {
		return nil
	}
	service, err := updating.New(e.ctx, e.Dir, *options, func(ctx context.Context, id string) (string, string, error) {
		database := filepath.Join(e.Dir, "backups", "before-update-"+id+".sqlite")
		configuration := filepath.Join(e.Dir, "backups", "before-update-"+id+".yaml")
		if err := e.DB.Backup(ctx, database); err != nil {
			return "", "", err
		}
		doc, err := e.ConfigStore.Read()
		if err != nil {
			return "", "", err
		}
		if err := securefs.AtomicWrite(configuration, []byte(doc.Text)); err != nil {
			return "", "", err
		}
		return database, configuration, nil
	}, e.Events.Publish)
	if err != nil {
		return err
	}
	e.Updates = service
	return nil
}
