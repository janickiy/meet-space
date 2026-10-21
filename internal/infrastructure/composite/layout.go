// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

// Пакетная компоновка захватывает и компонует медиаданные только для записи, независимо от
// переадресации SFU.
package composite

import (
	"math"
	"sort"

	"github.com/janickiy/meet-space/internal/domain/media"
)

// Tile — тайлер определяет один исходный прямоугольник в итоговом составном кадре.
type Tile struct {
	TrackID string `json:"trackId"`
	X       int    `json:"x"`
	Y       int    `json:"y"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
}

// Layout — компоновка определяет размеры итогового кадра и положение исходных тайлеров.
type Layout struct {
	Name   string `json:"name"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Tiles  []Tile `json:"tiles"`
}

// GridLayout вычисляет сетку составного видео для заданного количества видимых источников.
//
// @Parameters:
//   - tracks: Согласованные или архивированные дорожки, включенные в эту операцию.
//   - width: Ширина вывода в пикселях.
//   - height: Высота вывода в пикселях.
//
// @returns:
//   - result1: Итоговая компоновка.
func GridLayout(tracks []media.Track, width, height int) Layout {
	if width < 320 {
		width = 1280
	}
	if height < 240 {
		height = 720
	}
	width -= width % 2
	height -= height % 2
	result := Layout{Name: "grid", Width: width, Height: height, Tiles: []Tile{}}
	var cameras, screens []media.Track
	for _, track := range tracks {
		if track.Kind != media.KindVideo {
			continue
		}
		if track.Source == media.SourceVideoScreen {
			screens = append(screens, track)
		} else {
			cameras = append(cameras, track)
		}
	}

	less := func(items []media.Track) {
		sort.Slice(items,

			func(i, j int) bool {
				if items[i].ParticipantID == items[j].ParticipantID {
					return items[i].ID < items[j].ID
				}
				return items[i].ParticipantID < items[j].ParticipantID
			})
	}
	less(cameras)
	less(screens)
	if len(screens) > 0 {
		result.Name = "screen"
		mainWidth := width
		if len(cameras) > 0 {
			mainWidth = (width * 3 / 4) &^ 1
		}
		result.Tiles = append(result.Tiles, Tile{TrackID: screens[0].ID, Width: mainWidth, Height: height})
		result.Tiles = append(result.Tiles, gridTiles(cameras, mainWidth, 0, width-mainWidth, height)...)
		return result
	}
	result.Tiles = gridTiles(cameras, 0, 0, width, height)
	return result
}

// gridTiles создает геометрию для каждого тайлера, используемую фильтром составного видео.
//
// @Parameters:
//   - tracks: Согласованные или архивированные дорожки, включенные в эту операцию.
//   - x: Начало координат горизонтального тайлера в пикселях.
//   - y: Начало координат вертикального тайлера в пикселях.
//   - width: Ширина вывода в пикселях.
//   - height: Высота вывода в пикселях.
//
// @returns:
//   - result1: Собранные значения тайлеров.
func gridTiles(tracks []media.Track, x, y, width, height int) []Tile {
	result := make([]Tile, 0, len(tracks))
	if len(tracks) == 0 {
		return result
	}
	columns := int(math.Ceil(math.Sqrt(float64(len(tracks)))))
	if width < height {
		columns = int(math.Ceil(math.Sqrt(float64(len(tracks)) * float64(width) / float64(height))))
	}
	if columns < 1 {
		columns = 1
	}
	rows := (len(tracks) + columns - 1) / columns
	tileWidth, tileHeight := (width/columns)&^1, (height/rows)&^1
	for i, track := range tracks {
		result = append(result, Tile{TrackID: track.ID, X: x + (i%columns)*tileWidth, Y: y + (i/columns)*tileHeight, Width: tileWidth, Height: tileHeight})
	}
	return result
}
