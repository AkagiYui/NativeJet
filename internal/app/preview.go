package app

import (
	"nativejet/internal/ipc"
	"nativejet/internal/model"
)

type previewInput struct {
	client                  *ipc.Client
	profile, kind           string
	global, globalName      string
	source, base, overrides uint64
}

func (a *App) currentPreviewInput() previewInput {
	p := a.current()
	if p == nil {
		return previewInput{}
	}
	input := previewInput{
		client:    a.client,
		profile:   p.ID,
		kind:      p.Kind,
		global:    a.loadedGlobal,
		source:    a.source.Buffer.Version(),
		base:      a.global.Buffer.Version(),
		overrides: a.override.Buffer.Version(),
	}
	if preset := a.snapshot.State.GlobalOverrideByID(a.loadedGlobal); preset != nil {
		input.globalName = preset.Name
	}
	return input
}

func (a *App) refreshPreview(force bool) {
	if !a.canReadWindowData() {
		return
	}
	input := a.currentPreviewInput()
	if input.profile == "" || (!force && a.previewInput == input) {
		return
	}
	var preview model.Preview
	a.readWindowData(windowQuery{method: "config.preview"}, 0, a.draftParams(), &preview, func(err error) {
		if input != a.currentPreviewInput() {
			return
		}
		a.previewInput, a.previewError = input, err
		if err == nil {
			a.preview = preview
			a.final.Buffer.Reset(preview.YAML)
			a.field = 0
		}
	})
}
