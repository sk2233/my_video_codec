package main

import (
	"bytes"
	"compress/flate"
	"io"
	"log"
	"os"
)

// https://github.com/kevmo314/codec-from-scratch
// rgb->yuv => uv降采样 => 第一帧存储完整信息后续只存储差值 => 对差值采用游标编码进行压缩

func main() {
	// 编码
	Encode("video.rgb24", 384, 216, "video.data")
	// 解码
	//Decode("video.data", 384, 216, "video2.rgb24")
}

func Decode(inPath string, width, height int, outPath string) {
	file, err := os.Open(inPath)
	HandleErr(err)
	defer file.Close()
	// Now we have our encoded video. Let's decode it and see what we get.
	// First, we will decode the DEFLATE stream.
	var inflated bytes.Buffer
	r := flate.NewReader(file)
	if _, err = io.Copy(&inflated, r); err != nil {
		log.Fatal(err)
	}
	if err = r.Close(); err != nil {
		log.Fatal(err)
	}

	// Split the inflated stream into frames.
	decodedFrames := make([][]byte, 0)
	for {
		frame := make([]byte, width*height*3/2)
		if _, err = io.ReadFull(&inflated, frame); err != nil {
			if err == io.EOF {
				break
			}
			log.Fatal(err)
		}
		decodedFrames = append(decodedFrames, frame)
	}

	// For every frame except the first one, we need to add the previous frame to the delta frame.
	// This is the opposite of what we did in the encoder.
	for i := range decodedFrames {
		if i == 0 {
			continue
		}
		for j := 0; j < len(decodedFrames[i]); j++ {
			decodedFrames[i][j] += decodedFrames[i-1][j]
		}
	}

	// Then convert each YUV frame into RGB.
	for i, frame := range decodedFrames {
		Y := frame[:width*height]
		U := frame[width*height : width*height+width*height/4]
		V := frame[width*height+width*height/4:]
		rgb := make([]byte, 0, width*height*3)
		for j := 0; j < height; j++ {
			for k := 0; k < width; k++ {
				y := float64(Y[j*width+k])
				u := float64(U[(j/2)*(width/2)+(k/2)]) - 128
				v := float64(V[(j/2)*(width/2)+(k/2)]) - 128
				r := Clamp(y+1.402*v, 0, 255)
				g := Clamp(y-0.344*u-0.714*v, 0, 255)
				b := Clamp(y+1.772*u, 0, 255)
				rgb = append(rgb, uint8(r), uint8(g), uint8(b))
			}
		}
		decodedFrames[i] = rgb
	}
	buff := &bytes.Buffer{}
	for _, frame := range decodedFrames {
		buff.Write(frame)
	}
	err = os.WriteFile(outPath, buff.Bytes(), 0644)
	HandleErr(err)
}

func Encode(inpPath string, width, height int, outPath string) {
	frames := make([][]byte, 0)
	file, err := os.Open(inpPath)
	HandleErr(err)
	defer file.Close()
	for {
		// Read raw video frames from stdin. In rgb24 format, each pixel (r, g, b) is one byte
		// so the total Size of the frame is width * height * 3.
		frame := make([]byte, width*height*3)
		// read the frame from stdin
		if _, err = file.Read(frame); err != nil {
			break
		}
		frames = append(frames, frame)
	}
	// Now we have our raw video, using a truly ridiculous amount of memory!
	rawSize := Size(frames)
	log.Printf("Raw Size: %d bytes", rawSize)

	for i, frame := range frames {
		// First, we will convert each frame to YUV420 format. Each pixel in RGB24 format
		// looks like this:
		//
		// +-----------+-----------+-----------+-----------+
		// |           |           |           |           |
		// | (r, g, b) | (r, g, b) | (r, g, b) | (r, g, b) |
		// |           |           |           |           |
		// +-----------+-----------+-----------+-----------+
		// |           |           |           |           |
		// | (r, g, b) | (r, g, b) | (r, g, b) | (r, g, b) |
		// |           |           |           |           |
		// +-----------+-----------+-----------+-----------+  ...
		// |           |           |           |           |
		// | (r, g, b) | (r, g, b) | (r, g, b) | (r, g, b) |
		// |           |           |           |           |
		// +-----------+-----------+-----------+-----------+
		// |           |           |           |           |
		// | (r, g, b) | (r, g, b) | (r, g, b) | (r, g, b) |
		// |           |           |           |           |
		// +-----------+-----------+-----------+-----------+
		//
		//                        ...
		//
		// YUV420 format looks like this:
		//
		// +-----------+-----------+-----------+-----------+
		// |  Y(0, 0)  |  Y(0, 1)  |  Y(0, 2)  |  Y(0, 3)  |
		// |  U(0, 0)  |  U(0, 0)  |  U(0, 1)  |  U(0, 1)  |
		// |  V(0, 0)  |  V(0, 0)  |  V(0, 1)  |  V(0, 1)  |
		// +-----------+-----------+-----------+-----------+
		// |  Y(1, 0)  |  Y(1, 1)  |  Y(1, 2)  |  Y(1, 3)  |
		// |  U(0, 0)  |  U(0, 0)  |  U(0, 1)  |  U(0, 1)  |
		// |  V(0, 0)  |  V(0, 0)  |  V(0, 1)  |  V(0, 1)  |
		// +-----------+-----------+-----------+-----------+  ...
		// |  Y(2, 0)  |  Y(2, 1)  |  Y(2, 2)  |  Y(2, 3)  |
		// |  U(1, 0)  |  U(1, 0)  |  U(1, 1)  |  U(1, 1)  |
		// |  V(1, 0)  |  V(1, 0)  |  V(1, 1)  |  V(1, 1)  |
		// +-----------+-----------+-----------+-----------+
		// |  Y(3, 0)  |  Y(3, 1)  |  Y(3, 2)  |  Y(3, 3)  |
		// |  U(1, 0)  |  U(1, 0)  |  U(1, 1)  |  U(1, 1)  |
		// |  V(1, 0)  |  V(1, 0)  |  V(1, 1)  |  V(1, 1)  |
		// +-----------+-----------+-----------+-----------+
		//					      ...
		//
		// The gist of this format is that instead of the components R, G, B which each
		// pixel needs, we first convert it to a different space, Y (luminance) and UV (chrominance).
		// The way to think about this is that the Y component is the brightness of the pixel,
		// and the UV components are the color of the pixel. The UV components are shared
		// between 4 adjacent pixels, so we only need to store them once for each 4 pixels.
		//
		// The intuition is that the human eye is more sensitive to brightness than color,
		// so we can store the brightness of each pixel and then store the color of each
		// 4 pixels. This is a huge space savings, since we only need to store 1/4 of the
		// pixels in the image.
		//
		// If you're seeking more resources, YUV format is also known as YCbCr.
		// Actually that's not completely true, but it's close enough and color space selection
		// is a whole other topic.
		//
		// By convention, in our byte slice, we store reading left to right then top to bottom.
		// That is, to find a pixel at row i, column j, we would find the byte at index
		// (i * width + j) * 3.
		//
		// In practice, this doesn't matter that much because our image will be transposed if
		// this is done backwards. The important thing is that we are consistent.
		Y := make([]byte, width*height)
		U := make([]float64, width*height)
		V := make([]float64, width*height)
		for j := 0; j < width*height; j++ {
			// Convert the pixel from RGB to YUV
			r, g, b := float64(frame[3*j]), float64(frame[3*j+1]), float64(frame[3*j+2])
			// These coefficients are from the ITU-R standard.
			// See https://en.wikipedia.org/wiki/YUV#Y%E2%80%B2UV444_to_RGB888_conversion
			//
			// In practice, the actual coefficients vary based on the standard.
			// For our example, it doesn't matter that much, the key insight is
			// more that converting to YUV allows us to downsample the color
			// space efficiently.
			y := +0.299*r + 0.587*g + 0.114*b
			u := -0.169*r - 0.331*g + 0.449*b + 128
			v := 0.499*r - 0.418*g - 0.0813*b + 128
			// Store the YUV values in our byte slices. These are separated to make the
			// next step a bit easier.
			Y[j] = uint8(y)
			U[j] = u
			V[j] = v
		}
		// Now, we will downsample the U and V components. This is a process where we
		// take the 4 pixels that share a U and V component and average them together.
		// We will store the downsampled U and V components in these slices.
		uDownsampled := make([]byte, width*height/4) // 对 uv 降采样
		vDownsampled := make([]byte, width*height/4)
		for x := 0; x < height; x += 2 {
			for y := 0; y < width; y += 2 {
				// We will average the U and V components of the 4 pixels that share this
				// U and V component.  取对应的平均值
				u := (U[x*width+y] + U[x*width+y+1] + U[(x+1)*width+y] + U[(x+1)*width+y+1]) / 4
				v := (V[x*width+y] + V[x*width+y+1] + V[(x+1)*width+y] + V[(x+1)*width+y+1]) / 4
				// Store the downsampled U and V components in our byte slices.
				uDownsampled[x/2*width/2+y/2] = uint8(u)
				vDownsampled[x/2*width/2+y/2] = uint8(v)
			}
		}
		yuvFrame := make([]byte, len(Y)+len(uDownsampled)+len(vDownsampled))
		// Now we need to store the YUV values in a byte slice. To make the data more
		// compressible, we will store all the Y values first, then all the U values,
		// then all the V values. This is called a planar format.
		//
		// The intuition is that adjacent Y, U, and V values are more likely to be
		// similar than Y, U, and V themselves. Therefore, storing the components
		// in a planar format will save more data later.
		copy(yuvFrame, Y)
		copy(yuvFrame[len(Y):], uDownsampled)
		copy(yuvFrame[len(Y)+len(uDownsampled):], vDownsampled)
		frames[i] = yuvFrame // 处理完一帧
	}
	// Now we have our YUV-encoded video, which takes half the space!
	yuvSize := Size(frames)
	log.Printf("YUV420P Size: %d bytes (%0.2f%% original Size)", yuvSize, 100*float32(yuvSize)/float32(rawSize))

	// This is good, we're at 1/4 the Size of the original video. But we can do better.
	// Note that most of our longest runs are runs of zeros. This is because the delta
	// between frames is usually small. We have a bit of flexibility in choice of algorithm
	// here, so to keep the encoder simple, we will defer to using the DEFLATE algorithm
	// which is available in the standard library. The implementation is beyond the scope
	// of this demonstration.
	var deflated bytes.Buffer
	w, err := flate.NewWriter(&deflated, flate.BestCompression)
	if err != nil {
		log.Fatal(err)
	}
	for i := range frames {
		if i == 0 {
			// This is the keyframe, write the raw frame.
			if _, err = w.Write(frames[i]); err != nil {
				log.Fatal(err)
			}
			continue
		}
		delta := make([]byte, len(frames[i]))
		for j := 0; j < len(delta); j++ {
			delta[j] = frames[i][j] - frames[i-1][j]
		}
		if _, err = w.Write(delta); err != nil {
			log.Fatal(err)
		}
	}
	if err = w.Close(); err != nil {
		log.Fatal(err)
	}
	deflatedSize := deflated.Len()
	log.Printf("DEFLATE Size: %d bytes (%0.2f%% original Size)", deflatedSize, 100*float32(deflatedSize)/float32(rawSize))

	// You'll note that the DEFLATE step takes quite a while to run. In general, encoders tend to run
	// much slower than decoders. This is true for most compression algorithms, not just video codecs.
	// This is because the encoder needs to do a lot of work to analyze the data and make decisions
	// about how to compress it. The decoder, on the other hand, is just a simple loop that reads the
	// data and does the opposite of the encoder.
	//
	// At this point, we've achieved a 90% compression ratio!
	//
	// As an aside, you might be thinking that typical JPEG compression is 90%, so why not JPEG encode
	// every frame? While true, the algorithm we have supplied above is quite a bit simpler than JPEG.
	// We demonstrate that taking advantage of temporal locality can yield compression ratios just as
	// high as JPEG, but with a much simpler algorithm.
	//
	// Additionally, the DEFLATE algorithm does not take advantage of the two dimensionality of the data
	// and is therefore not as efficient as it could be. In the real world, video codecs are much more
	// complex than the one we have implemented here. They take advantage of the two dimensionality of
	// the data, they use more sophisticated algorithms, and they are optimized for the hardware they
	// run on. For example, the H.264 codec is implemented in hardware on many modern GPUs.
	err = os.WriteFile(outPath, deflated.Bytes(), 0644)
	HandleErr(err)
}

func Size(frames [][]byte) int {
	var res int
	for _, frame := range frames {
		res += len(frame)
	}
	return res
}

func Clamp(x, min, max float64) float64 {
	if x < min {
		return min
	}
	if x > max {
		return max
	}
	return x
}
