//! 采掘裂纹 overlay pass:Go `block_crack.go`/`frame_streams.go` 的 GPU 侧
//! 呈现。
//!
//! Go 侧把当前权威采掘目标编码为恰 1 个 80 字节实例(mat4 + atlas 层号
//! f32 + 零填充),经 frame v2 的 TLV tag 10 段过境;本模块以固定容量的
//! 常驻 pass 绘制带 UV 的单位立方体,六面采样同一裂纹层,呈现为原方块
//! 材质之上的透明 cutout。管线状态镜像 `EntityPass` 的轮廓变体:alpha
//! blend、深度只读、`CompareFunction::LessEqual`、无背面剔除——配合
//! Go 侧 0.001 外扩(合成边长 1.002)防与方块表面的深度冲突。
//!
//! 边界(`voxel-visual-presentation` delta 钉死):实例容量恰为 1、复用
//! 方块材质 atlas(无独立纹理上传入口,bind 随 atlas 上传重建)、无每帧
//! 动态资源创建、不写深度附件、无透明排序。

use super::shaders;

/// 每实例字节数:mat4(64)+ f32 atlas 层号(4)+ 12 字节零填充,与 Go
/// `EncodeBlockCrackInstances` 的跨语言契约一致。
pub const CRACK_INSTANCE_BYTES: usize = 80;
/// 实例容量,被规格钉死为恰 1:单一可复用 overlay,第二个裂纹 overlay 或
/// 残留实例都视为违约。
pub const CRACK_MAX_INSTANCES: usize = 1;
/// camera uniform 字节数与 instances 的缓冲偏移(uniform 对齐 256),布局
/// 与 `entity.rs` 的 dynamic 缓冲同构:camera 80B @0、instances @256、
/// indirect 20B 随后。
const CAMERA_BYTES: usize = 80;
const INSTANCE_OFFSET: usize = 256;
const INDIRECT_BYTES: usize = 20;
/// 立方体索引数(6 面 × 6 索引)。
const CUBE_INDEX_COUNT: u32 = 36;
/// 顶点步长:pos 3f + uv 2f = 20 字节。
const VERTEX_STRIDE: usize = 20;

/// 与 `entity.rs` 的 `CUBE_VERTICES` 同面序(±X、±Y、±Z)的 24 顶点单位
/// 立方体位置(每面 4 顶点);面序一致让本模块与实体几何共享同一套心智
/// 模型,差异只在补齐 UV。
#[rustfmt::skip]
const CUBE_POSITIONS: [f32; 72] = [
    // +X
    0.5, -0.5, -0.5, 0.5, 0.5, -0.5, 0.5, 0.5, 0.5, 0.5, -0.5, 0.5,
    // -X
    -0.5, -0.5, 0.5, -0.5, 0.5, 0.5, -0.5, 0.5, -0.5, -0.5, -0.5, -0.5,
    // +Y
    -0.5, 0.5, -0.5, -0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, -0.5,
    // -Y
    -0.5, -0.5, 0.5, -0.5, -0.5, -0.5, 0.5, -0.5, -0.5, 0.5, -0.5, 0.5,
    // +Z
    0.5, -0.5, 0.5, 0.5, 0.5, 0.5, -0.5, 0.5, 0.5, -0.5, -0.5, 0.5,
    // -Z
    -0.5, -0.5, -0.5, -0.5, 0.5, -0.5, 0.5, 0.5, -0.5, 0.5, -0.5, -0.5,
];

/// 与 [`CUBE_POSITIONS`] 同序的逐顶点 UV,每面覆盖 0..1 全图。侧面的顶点
/// 环绕是「底、顶、顶、底」,纹理 v=0(图像顶行)对应方块顶部,与 terrain
/// 侧面「-world.y 为纵向」的朝向约定一致;顶/底面四点同高,朝向无语义,
/// 按环绕序铺满即可——裂纹六面同图案,面间镜像/旋转不可分辨。
#[rustfmt::skip]
const CUBE_UVS: [f32; 48] = [
    // +X:底、顶、顶、底
    0.0, 1.0, 0.0, 0.0, 1.0, 0.0, 1.0, 1.0,
    // -X
    0.0, 1.0, 0.0, 0.0, 1.0, 0.0, 1.0, 1.0,
    // +Y:环绕序铺满
    0.0, 0.0, 0.0, 1.0, 1.0, 1.0, 1.0, 0.0,
    // -Y
    0.0, 0.0, 0.0, 1.0, 1.0, 1.0, 1.0, 0.0,
    // +Z
    0.0, 1.0, 0.0, 0.0, 1.0, 0.0, 1.0, 1.0,
    // -Z
    0.0, 1.0, 0.0, 0.0, 1.0, 0.0, 1.0, 1.0,
];

/// 与 `entity.rs` 的 `CUBE_INDICES` 一致的索引(每面两个三角形)。
#[rustfmt::skip]
const CUBE_INDICES: [u32; 36] = [
    0, 1, 2, 0, 2, 3,
    4, 5, 6, 4, 6, 7,
    8, 9, 10, 8, 10, 11,
    12, 13, 14, 12, 14, 15,
    16, 17, 18, 16, 18, 19,
    20, 21, 22, 20, 22, 23,
];

/// 采掘裂纹 pass 的全部 GPU 资源。bind group 在材质 atlas 上传后经
/// [`CrackPass::rebuild_bind`] 建立(与 `lod_pass.rebuild_bind` 同一先例):
/// camera 与 instances 缓冲是常驻的,atlas 未上传前仅缺纹理绑定。
pub struct CrackPass {
    dynamic: wgpu::Buffer,
    vertices: wgpu::Buffer,
    indices: wgpu::Buffer,
    pipeline: wgpu::RenderPipeline,
    /// atlas 视图 + sampler 绑定;None 表示 atlas 尚未上传,本帧不绘制。
    bind: Option<wgpu::BindGroup>,
    indirect_offset: usize,
}

impl CrackPass {
    /// 创建 pass:容量固定为 [`CRACK_MAX_INSTANCES`],管线为透明 cutout
    /// 变体(alpha blend + 深度只读 + LessEqual + 无剔除)。
    pub fn new(
        device: &wgpu::Device,
        queue: &wgpu::Queue,
        color_format: wgpu::TextureFormat,
        depth_format: wgpu::TextureFormat,
    ) -> Self {
        let instance_size = CRACK_MAX_INSTANCES * CRACK_INSTANCE_BYTES;
        let indirect_offset = INSTANCE_OFFSET + instance_size;
        let upload_bytes = indirect_offset + INDIRECT_BYTES;
        let dynamic = device.create_buffer(&wgpu::BufferDescriptor {
            label: Some("crack overlay"),
            size: upload_bytes as u64,
            usage: wgpu::BufferUsages::UNIFORM
                | wgpu::BufferUsages::STORAGE
                | wgpu::BufferUsages::INDIRECT
                | wgpu::BufferUsages::COPY_DST,
            mapped_at_creation: false,
        });
        // 顶点缓冲:24 顶点交错 pos(3f)+uv(2f),构造时一次写入。
        let mut vertex_bytes = Vec::with_capacity(24 * VERTEX_STRIDE);
        for vertex in 0..24usize {
            for value in &CUBE_POSITIONS[vertex * 3..vertex * 3 + 3] {
                vertex_bytes.extend_from_slice(&value.to_le_bytes());
            }
            for value in &CUBE_UVS[vertex * 2..vertex * 2 + 2] {
                vertex_bytes.extend_from_slice(&value.to_le_bytes());
            }
        }
        let vertices = device.create_buffer(&wgpu::BufferDescriptor {
            label: Some("crack cube vertices"),
            size: vertex_bytes.len() as u64,
            usage: wgpu::BufferUsages::VERTEX | wgpu::BufferUsages::COPY_DST,
            mapped_at_creation: false,
        });
        queue.write_buffer(&vertices, 0, &vertex_bytes);
        let indices = device.create_buffer(&wgpu::BufferDescriptor {
            label: Some("crack cube indices"),
            size: (CUBE_INDICES.len() * 4) as u64,
            usage: wgpu::BufferUsages::INDEX | wgpu::BufferUsages::COPY_DST,
            mapped_at_creation: false,
        });
        let mut index_bytes = Vec::with_capacity(CUBE_INDICES.len() * 4);
        for v in CUBE_INDICES {
            index_bytes.extend_from_slice(&v.to_le_bytes());
        }
        queue.write_buffer(&indices, 0, &index_bytes);

        let layout = device.create_bind_group_layout(&wgpu::BindGroupLayoutDescriptor {
            label: Some("crack layout"),
            entries: &[
                wgpu::BindGroupLayoutEntry {
                    binding: 0,
                    visibility: wgpu::ShaderStages::VERTEX,
                    ty: wgpu::BindingType::Buffer {
                        ty: wgpu::BufferBindingType::Uniform,
                        has_dynamic_offset: false,
                        min_binding_size: None,
                    },
                    count: None,
                },
                wgpu::BindGroupLayoutEntry {
                    binding: 1,
                    // 仅 vertex 阶段读取实例(镜像 EntityPass 先例):shader
                    // 的 fs_main 只消费 vs_main 传出的 uv/layer/daylight。
                    visibility: wgpu::ShaderStages::VERTEX,
                    ty: wgpu::BindingType::Buffer {
                        ty: wgpu::BufferBindingType::Storage { read_only: true },
                        has_dynamic_offset: false,
                        min_binding_size: None,
                    },
                    count: None,
                },
                wgpu::BindGroupLayoutEntry {
                    binding: 2,
                    visibility: wgpu::ShaderStages::FRAGMENT,
                    ty: wgpu::BindingType::Texture {
                        sample_type: wgpu::TextureSampleType::Float { filterable: true },
                        view_dimension: wgpu::TextureViewDimension::D2Array,
                        multisampled: false,
                    },
                    count: None,
                },
                wgpu::BindGroupLayoutEntry {
                    binding: 3,
                    visibility: wgpu::ShaderStages::FRAGMENT,
                    ty: wgpu::BindingType::Sampler(wgpu::SamplerBindingType::Filtering),
                    count: None,
                },
            ],
        });
        let module = device.create_shader_module(wgpu::ShaderModuleDescriptor {
            label: Some("crack"),
            source: wgpu::ShaderSource::Wgsl(shaders::CRACK.into()),
        });
        let pipeline_layout = device.create_pipeline_layout(&wgpu::PipelineLayoutDescriptor {
            label: None,
            bind_group_layouts: &[Some(&layout)],
            immediate_size: 0,
        });
        let pipeline = device.create_render_pipeline(&wgpu::RenderPipelineDescriptor {
            label: Some("crack overlay"),
            layout: Some(&pipeline_layout),
            vertex: wgpu::VertexState {
                module: &module,
                entry_point: Some("vs_main"),
                compilation_options: Default::default(),
                buffers: &[wgpu::VertexBufferLayout {
                    array_stride: VERTEX_STRIDE as u64,
                    step_mode: wgpu::VertexStepMode::Vertex,
                    attributes: &[
                        wgpu::VertexAttribute {
                            format: wgpu::VertexFormat::Float32x3,
                            offset: 0,
                            shader_location: 0,
                        },
                        wgpu::VertexAttribute {
                            format: wgpu::VertexFormat::Float32x2,
                            offset: 8,
                            shader_location: 1,
                        },
                    ],
                }],
            },
            fragment: Some(wgpu::FragmentState {
                module: &module,
                entry_point: Some("fs_main"),
                compilation_options: Default::default(),
                targets: &[Some(wgpu::ColorTargetState {
                    format: color_format,
                    // alpha blend:裂纹是原方块材质之上的半透明叠加阶段。
                    blend: Some(wgpu::BlendState::ALPHA_BLENDING),
                    write_mask: wgpu::ColorWrites::ALL,
                })],
            }),
            primitive: wgpu::PrimitiveState {
                topology: wgpu::PrimitiveTopology::TriangleList,
                front_face: wgpu::FrontFace::Ccw,
                // 无剔除:overlay 立方体从内部看同样要呈现裂纹(相机贴近
                // 目标方块时视线可能进入外扩后的包围盒)。
                cull_mode: None,
                ..Default::default()
            },
            depth_stencil: Some(wgpu::DepthStencilState {
                format: depth_format,
                // 深度只读 + LessEqual:被地形遮挡的部分不穿透,同时外扩的
                // overlay 能贴着方块表面通过深度测试(镜像轮廓 pass 先例)。
                depth_write_enabled: Some(false),
                depth_compare: Some(wgpu::CompareFunction::LessEqual),
                stencil: wgpu::StencilState::default(),
                bias: wgpu::DepthBiasState::default(),
            }),
            multisample: wgpu::MultisampleState::default(),
            multiview_mask: None,
            cache: None,
        });
        Self {
            dynamic,
            vertices,
            indices,
            pipeline,
            bind: None,
            indirect_offset,
        }
    }

    /// (重)建 atlas 绑定:直接复用调用方传入的方块 atlas 视图与采样器,
    /// 与 terrain/water/lod 同一图集。atlas 每次上传都整体替换,bind 随之
    /// 重建;调用方是 `OffscreenRenderer::upload_atlas` 末尾。
    pub fn rebuild_bind(
        &mut self,
        device: &wgpu::Device,
        atlas_view: &wgpu::TextureView,
        sampler: &wgpu::Sampler,
    ) {
        self.bind = Some(
            device.create_bind_group(&wgpu::BindGroupDescriptor {
                label: Some("crack resources"),
                layout: &self.pipeline.get_bind_group_layout(0),
                entries: &[
                    wgpu::BindGroupEntry {
                        binding: 0,
                        resource: wgpu::BindingResource::Buffer(wgpu::BufferBinding {
                            buffer: &self.dynamic,
                            offset: 0,
                            size: Some((CAMERA_BYTES as u64).try_into().unwrap()),
                        }),
                    },
                    wgpu::BindGroupEntry {
                        binding: 1,
                        resource: wgpu::BindingResource::Buffer(wgpu::BufferBinding {
                            buffer: &self.dynamic,
                            offset: INSTANCE_OFFSET as u64,
                            size: Some(
                                ((CRACK_MAX_INSTANCES * CRACK_INSTANCE_BYTES) as u64)
                                    .try_into()
                                    .unwrap(),
                            ),
                        }),
                    },
                    wgpu::BindGroupEntry {
                        binding: 2,
                        resource: wgpu::BindingResource::TextureView(atlas_view),
                    },
                    wgpu::BindGroupEntry {
                        binding: 3,
                        resource: wgpu::BindingResource::Sampler(sampler),
                    },
                ],
            }),
        );
    }

    /// 校验 instance 段字节:80 的倍数且不超过容量(恰 1 实例)。容量是
    /// 编译期常量,校验无需 GPU 资源,因此做成关联函数供无适配器环境的
    /// 单元测试直接复用。
    pub fn instances_valid(instances: &[u8]) -> bool {
        instances.len().is_multiple_of(CRACK_INSTANCE_BYTES)
            && instances.len() <= CRACK_MAX_INSTANCES * CRACK_INSTANCE_BYTES
    }

    /// 上传 camera(80B,布局与 `entity.rs` 的 upload 同构:viewproj +
    /// daylight + 零填充)与 instances,写 indirect 参数。
    pub fn upload(
        &self,
        queue: &wgpu::Queue,
        view_proj: &[f32; 16],
        daylight: f32,
        instances: &[u8],
    ) {
        debug_assert!(Self::instances_valid(instances));
        let mut camera = [0u8; CAMERA_BYTES];
        for (i, v) in view_proj.iter().enumerate() {
            camera[i * 4..i * 4 + 4].copy_from_slice(&v.to_le_bytes());
        }
        camera[64..68].copy_from_slice(&daylight.to_le_bytes());
        queue.write_buffer(&self.dynamic, 0, &camera);
        queue.write_buffer(&self.dynamic, INSTANCE_OFFSET as u64, instances);
        let count = (instances.len() / CRACK_INSTANCE_BYTES) as u32;
        let mut indirect = [0u8; INDIRECT_BYTES];
        indirect[0..4].copy_from_slice(&CUBE_INDEX_COUNT.to_le_bytes());
        indirect[4..8].copy_from_slice(&count.to_le_bytes());
        queue.write_buffer(&self.dynamic, self.indirect_offset as u64, &indirect);
    }

    /// 在已有颜色/深度附件之上录制本 pass(LoadOp::Load)。atlas 未上传时
    /// 直接返回、不开始 render pass——与 `terrain_bind == None` 的跳过同
    /// 语义:裂纹是可选呈现,资源未就绪的帧退化为无裂纹而非报错。
    pub fn record(
        &self,
        encoder: &mut wgpu::CommandEncoder,
        color: &wgpu::TextureView,
        depth: &wgpu::TextureView,
        label: &str,
    ) {
        let Some(bind) = &self.bind else {
            return;
        };
        let mut pass = encoder.begin_render_pass(&wgpu::RenderPassDescriptor {
            label: Some(label),
            color_attachments: &[Some(wgpu::RenderPassColorAttachment {
                view: color,
                depth_slice: None,
                resolve_target: None,
                ops: wgpu::Operations {
                    load: wgpu::LoadOp::Load,
                    store: wgpu::StoreOp::Store,
                },
            })],
            depth_stencil_attachment: Some(wgpu::RenderPassDepthStencilAttachment {
                view: depth,
                // 深度只读:load 后原样 store,本 pass 不改写深度附件。
                depth_ops: Some(wgpu::Operations {
                    load: wgpu::LoadOp::Load,
                    store: wgpu::StoreOp::Store,
                }),
                stencil_ops: None,
            }),
            occlusion_query_set: None,
            timestamp_writes: None,
            multiview_mask: None,
        });
        pass.set_pipeline(&self.pipeline);
        pass.set_bind_group(0, bind, &[]);
        pass.set_vertex_buffer(0, self.vertices.slice(..));
        pass.set_index_buffer(self.indices.slice(..), wgpu::IndexFormat::Uint32);
        pass.draw_indexed_indirect(&self.dynamic, self.indirect_offset as u64);
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    /// 实例段校验锁:80 的倍数且不超过容量(恰 1 实例);空流合法(本帧
    /// 无裂纹),79 字节(错位)与 160 字节(第二个实例)都必须拒绝。
    #[test]
    fn instances_valid_locks_layout_and_capacity() {
        assert!(CrackPass::instances_valid(&[]), "空流合法(本帧无裂纹)");
        assert!(CrackPass::instances_valid(&[0u8; 80]), "恰 1 实例合法");
        assert!(
            !CrackPass::instances_valid(&[0u8; 79]),
            "非 80 倍数必须拒绝"
        );
        assert!(
            !CrackPass::instances_valid(&[0u8; 160]),
            "超出容量 1 实例必须拒绝"
        );
    }

    /// 容量被规格钉死为恰 1:单一可复用 overlay,不允许第二个裂纹 overlay
    /// 或残留实例。
    #[test]
    fn capacity_is_exactly_one() {
        assert_eq!(CRACK_MAX_INSTANCES, 1);
    }

    /// 几何常量完整性:24 顶点(位置与 UV 各一份)、36 索引、20 字节顶点
    /// 步长;索引不得引用越界顶点。
    #[test]
    fn geometry_constants_are_complete() {
        assert_eq!(CUBE_POSITIONS.len(), 24 * 3, "24 顶点的位置分量");
        assert_eq!(CUBE_UVS.len(), 24 * 2, "24 顶点的 UV 分量");
        assert_eq!(CUBE_INDICES.len(), 36, "6 面 × 6 索引");
        assert_eq!(VERTEX_STRIDE, 20, "pos 3f + uv 2f = 20 字节");
        assert!(CUBE_INDICES.iter().all(|&i| i < 24), "索引不得越界");
    }
}
