### 一个简单的视频编解码器
rgb->yuv => uv降采样 => 第一帧存储完整信息后续只存储差值 => 对差值采用游标编码进行压缩<br>
Encode   编码<br>
Decode   解码<br>
参考：https://github.com/kevmo314/codec-from-scratch
