package photoimage

import (
	"bytes"
	"encoding/binary"
	"image"
)

// JPEG orientation is applied before EXIF is removed; offsets remain inside one APP1 segment.
func orientation(data []byte) int {
	if len(data) < 2 || data[0] != 0xff || data[1] != 0xd8 {
		return 1
	}
	for pos := 2; pos+4 <= len(data); {
		if data[pos] != 0xff {
			return 1
		}
		marker := data[pos+1]
		pos += 2
		if marker == 0xda || marker == 0xd9 {
			return 1
		}
		if marker == 0x00 || marker == 0xff {
			return 1
		}
		size := int(binary.BigEndian.Uint16(data[pos : pos+2]))
		if size < 2 || size > len(data)-pos {
			return 1
		}
		segment := data[pos+2 : pos+size]
		pos += size
		if marker != 0xe1 || len(segment) < 14 || !bytes.Equal(segment[:6], []byte{'E', 'x', 'i', 'f', 0, 0}) {
			continue
		}
		tiff := segment[6:]
		var order binary.ByteOrder
		switch string(tiff[:2]) {
		case "II":
			order = binary.LittleEndian
		case "MM":
			order = binary.BigEndian
		default:
			continue
		}
		if order.Uint16(tiff[2:4]) != 42 {
			continue
		}
		offset := uint64(order.Uint32(tiff[4:8]))
		if offset > uint64(len(tiff)-2) {
			continue
		}
		start := int(offset)
		count := int(order.Uint16(tiff[start : start+2]))
		start += 2
		if count > (len(tiff)-start)/12 {
			continue
		}
		for i := 0; i < count; i++ {
			entry := tiff[start+i*12 : start+(i+1)*12]
			if order.Uint16(entry[:2]) == 0x112 && order.Uint16(entry[2:4]) == 3 && order.Uint32(entry[4:8]) == 1 {
				value := int(order.Uint16(entry[8:10]))
				if value >= 1 && value <= 8 {
					return value
				}
			}
		}
	}
	return 1
}
func orient(src image.Image, value int) image.Image {
	if value == 1 {
		return src
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	width, height := w, h
	if value >= 5 {
		width, height = h, w
	}
	dst := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx, dy := x, y
			switch value {
			case 2:
				dx = w - 1 - x
			case 3:
				dx, dy = w-1-x, h-1-y
			case 4:
				dy = h - 1 - y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = h-1-y, x
			case 7:
				dx, dy = h-1-y, w-1-x
			case 8:
				dx, dy = y, w-1-x
			}
			dst.Set(dx, dy, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}
