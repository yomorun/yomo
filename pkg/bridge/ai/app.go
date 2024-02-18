package ai

// import (
// 	"errors"
// 	"sync"

// 	"github.com/yomorun/yomo"
// )

// var (
// 	apps                 sync.Map
// 	ErrInvalidApp        = errors.New("invalid app")
// 	ErrInvalidCredential = errors.New("invalid credential")
// )

// // App is the ai application
// type App struct {
// 	ID          string      `json:"id"`
// 	Credentials string      `json:"credentials"`
// 	Source      yomo.Source `json:"source"`
// }

// // GetOrCreateApp get or create app by appID, if app is created, it will connect to yomo zipper with credential.
// func (a *AIServer) GetOrCreateApp(appID string, credential string) (*App, error) {
// 	res, ok := apps.LoadOrStore(appID, &App{
// 		ID:          appID,
// 		Credentials: credential,
// 	})

// 	if !ok {
// 		// connect to yomo zipper when created
// 		err := res.(*App).ConnectToZipper(a.Name, a.ZipperAddr, credential)
// 		// if can not connect to yomo zipper, remove this app
// 		if err != nil {
// 			apps.Delete(appID)
// 			return nil, err
// 		}
// 	}

// 	app, ok := res.(*App)
// 	if !ok {
// 		return nil, ErrInvalidApp
// 	}
// 	if app.Credentials != credential {
// 		return nil, ErrInvalidCredential
// 	}

// 	return res.(*App), nil
// }

// // ConnectToZipper is used to connect to yomo zipper with credential
// func (app *App) ConnectToZipper(name string, zipperAddr string, credential string) error {
// 	source := yomo.NewSource(
// 		name,
// 		zipperAddr,
// 		yomo.WithSourceReConnect(),
// 		yomo.WithCredential(credential),
// 	)
// 	// create ai source
// 	err := source.Connect()
// 	if err != nil {
// 		return err
// 	}
// 	// set app source
// 	app.Source = source
// 	return nil
// }
