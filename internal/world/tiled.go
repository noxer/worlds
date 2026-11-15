package world

import (
	"encoding/json"
	"fmt"
	"io/fs"
)

// This file contains the minimal subset of the Tiled JSON formats (.tmj maps,
// .tsj tilesets and .world files) that the game understands.

const (
	gidFlipH    = 0x80000000
	gidFlipV    = 0x40000000
	gidFlipDiag = 0x20000000
	gidMask     = ^uint32(gidFlipH | gidFlipV | gidFlipDiag)
)

type tiledProperty struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value any    `json:"value"`
}

type tiledMap struct {
	Width      int               `json:"width"`
	Height     int               `json:"height"`
	TileWidth  int               `json:"tilewidth"`
	TileHeight int               `json:"tileheight"`
	Infinite   bool              `json:"infinite"`
	Layers     []tiledLayer      `json:"layers"`
	Tilesets   []tiledTilesetRef `json:"tilesets"`
	Properties []tiledProperty   `json:"properties"`
}

type tiledLayer struct {
	Name       string          `json:"name"`
	Type       string          `json:"type"`
	Width      int             `json:"width"`
	Height     int             `json:"height"`
	Data       []uint32        `json:"data"`
	Visible    bool            `json:"visible"`
	Objects    []tiledObject   `json:"objects"`
	Layers     []tiledLayer    `json:"layers"` // group layers
	Properties []tiledProperty `json:"properties"`
}

type tiledObject struct {
	ID         int             `json:"id"`
	Name       string          `json:"name"`
	Type       string          `json:"type"`
	Class      string          `json:"class"` // Tiled 1.9 used "class" instead of "type"
	X          float64         `json:"x"`
	Y          float64         `json:"y"`
	Width      float64         `json:"width"`
	Height     float64         `json:"height"`
	GID        uint32          `json:"gid"`
	Point      bool            `json:"point"`
	Properties []tiledProperty `json:"properties"`
}

type tiledTilesetRef struct {
	FirstGID uint32 `json:"firstgid"`
	Source   string `json:"source"`
}

type tiledTileset struct {
	Name       string `json:"name"`
	TileWidth  int    `json:"tilewidth"`
	TileHeight int    `json:"tileheight"`
	TileCount  int    `json:"tilecount"`
	Columns    int    `json:"columns"`
	Image      string `json:"image"`
	Margin     int    `json:"margin"`
	Spacing    int    `json:"spacing"`
	Tiles      []struct {
		ID         int             `json:"id"`
		Properties []tiledProperty `json:"properties"`
		Animation  []struct {
			TileID   int `json:"tileid"`
			Duration int `json:"duration"` // milliseconds
		} `json:"animation"`
	} `json:"tiles"`
}

type tiledWorld struct {
	Maps []struct {
		FileName string `json:"fileName"`
		X        int    `json:"x"`
		Y        int    `json:"y"`
		Width    int    `json:"width"`
		Height   int    `json:"height"`
	} `json:"maps"`
}

func readJSON(fsys fs.FS, name string, v any) error {
	b, err := fs.ReadFile(fsys, name)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// Props is a set of custom properties from Tiled.
type Props map[string]any

func toProps(ps []tiledProperty) Props {
	if len(ps) == 0 {
		return nil
	}
	m := make(Props, len(ps))
	for _, p := range ps {
		m[p.Name] = p.Value
	}
	return m
}

// String returns a string property or "".
func (p Props) String(name string) string {
	s, _ := p[name].(string)
	return s
}

// Bool returns a bool property or false.
func (p Props) Bool(name string) bool {
	switch v := p[name].(type) {
	case bool:
		return v
	case string:
		return v == "true"
	}
	return false
}

// Int returns an int property or def.
func (p Props) Int(name string, def int) int {
	if v, ok := p[name].(float64); ok {
		return int(v)
	}
	return def
}
