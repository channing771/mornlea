//! 实体 pass(avatar 与掉落物共用):Go `avatar.go`/`drop.go` 的 GPU 侧
//! 逐语义移植。
//!
//! CPU 准备(排序、部件构建、动画相位)留在 Go,96 字节/实例的字节流经
//! frame v2 pass 段过境;本模块只负责与 Go 相同的 dynamic 缓冲布局
//! (camera 80B @0、instances @256、indirect 20B 随后)、立方体几何与
//! indexed indirect 绘制。两个 pass 仅容量不同:avatar 450 实例(75 具身体
//! × 6 部件,玩家+伙伴+敌怪+被动牛合计),掉落物 800 实例。
//!
//! 实例布局(与 Go `avatarInstanceBytes` 逐字节一致):mat4(0..64)+
//! RGBA color(64..80)+ 材质 u32(80..84)+ 12 字节保留零填充(84..96)。
//! 纯色分支传哨兵材质走原纯色路径,牛身传牛皮/牛头层走贴图路径。

/// 每实例字节数:mat4(64)+ RGBA color(16)+ 材质 u32(4)+ 保留零填充(12),
/// 与 Go `avatarInstanceBytes` 一致。
pub const ENTITY_INSTANCE_BYTES: usize = 96;
/// 纯色分支的哨兵材质:与全部有效材质层号不相交,着色器据此走原纯色路径。
pub const AVATAR_MATERIAL_SOLID: u32 = u32::MAX;
/// 实例内材质槽的字节偏移(小端 u32)。
pub const AVATAR_MATERIAL_OFFSET: usize = 80;
/// avatar 实例容量(75 具身体 × 6 部件),与 Go `render.maxAvatars` 同源。
pub const AVATAR_MAX_INSTANCES: usize = 450;
/// 掉落物实例容量(core.MaxSessionDrops)。
pub const DROP_MAX_INSTANCES: usize = 800;
/// camera uniform 字节数与 instances 的缓冲偏移(uniform 对齐 256)。
const CAMERA_BYTES: usize = 80;
const INSTANCE_OFFSET: usize = 256;
const INDIRECT_BYTES: usize = 20;
/// 立方体索引数(6 面 × 6 索引)。
const CUBE_INDEX_COUNT: u32 = 36;

/// 与 Go `avatarCubeVertices` 逐字一致的 24 顶点立方体(每面 4 顶点)。
#[rustfmt::skip]
const CUBE_VERTICES: [f32; 72] = [
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

/// 与 Go `avatarCubeIndices` 一致的索引。
#[rustfmt::skip]
const CUBE_INDICES: [u32; 36] = [
    0, 1, 2, 0, 2, 3,
    4, 5, 6, 4, 6, 7,
    8, 9, 10, 8, 10, 11,
    12, 13, 14, 12, 14, 15,
    16, 17, 18, 16, 18, 19,
    20, 21, 22, 20, 22, 23,
];

/// 实体管线状态变体:avatar/drop 用不透明写深度,轮廓用透明只读深度
/// (LessEqual),镜像 Go 各构造参数。
#[derive(Clone, Copy)]
pub enum EntityPipelineKind {
    /// BlendReplace + 深度写 + Less。
    Opaque,
    /// BlendAlpha + 深度只读 + LessEqual(方块轮廓)。
    OutlineTranslucent,
}

/// 一个实体 pass 的全部 GPU 资源。
pub struct EntityPass {
    dynamic: wgpu::Buffer,
    vertices: wgpu::Buffer,
    indices: wgpu::Buffer,
    pipeline: wgpu::RenderPipeline,
    /// atlas 视图 + sampler 绑定;None 表示 atlas 尚未上传,本帧不绘制
    /// (与 `terrain_bind == None` 及裂纹 pass 同语义:预热后恒为 Some,
    /// 帧内不再创建任何绑定资源)。
    bind: Option<wgpu::BindGroup>,
    instance_size: usize,
    indirect_offset: usize,
}

impl EntityPass {
    /// 创建 pass;`max_instances` 决定 dynamic 缓冲布局(镜像 Go 常量)。
    #[allow(clippy::too_many_arguments)]
    pub fn new(
        device: &wgpu::Device,
        queue: &wgpu::Queue,
        shader: &wgpu::ShaderModule,
        label: &str,
        max_instances: usize,
        kind: EntityPipelineKind,
        color_format: wgpu::TextureFormat,
        depth_format: wgpu::TextureFormat,
    ) -> Self {
        let (blend, depth_write, depth_compare) = match kind {
            EntityPipelineKind::Opaque => {
                (wgpu::BlendState::REPLACE, true, wgpu::CompareFunction::Less)
            }
            EntityPipelineKind::OutlineTranslucent => (
                wgpu::BlendState::ALPHA_BLENDING,
                false,
                wgpu::CompareFunction::LessEqual,
            ),
        };
        let instance_size = max_instances * ENTITY_INSTANCE_BYTES;
        let indirect_offset = INSTANCE_OFFSET + instance_size;
        let upload_bytes = indirect_offset + INDIRECT_BYTES;
        let dynamic = device.create_buffer(&wgpu::BufferDescriptor {
            label: Some(label),
            size: upload_bytes as u64,
            usage: wgpu::BufferUsages::UNIFORM
                | wgpu::BufferUsages::STORAGE
                | wgpu::BufferUsages::INDIRECT
                | wgpu::BufferUsages::COPY_DST,
            mapped_at_creation: false,
        });
        let vertices = device.create_buffer(&wgpu::BufferDescriptor {
            label: Some("entity cube vertices"),
            size: (CUBE_VERTICES.len() * 4) as u64,
            usage: wgpu::BufferUsages::VERTEX | wgpu::BufferUsages::COPY_DST,
            mapped_at_creation: false,
        });
        let mut vertex_bytes = Vec::with_capacity(CUBE_VERTICES.len() * 4);
        for v in CUBE_VERTICES {
            vertex_bytes.extend_from_slice(&v.to_le_bytes());
        }
        queue.write_buffer(&vertices, 0, &vertex_bytes);
        let indices = device.create_buffer(&wgpu::BufferDescriptor {
            label: Some("entity cube indices"),
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
            label: Some("entity layout"),
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
        let pipeline_layout = device.create_pipeline_layout(&wgpu::PipelineLayoutDescriptor {
            label: None,
            bind_group_layouts: &[Some(&layout)],
            immediate_size: 0,
        });
        let pipeline = device.create_render_pipeline(&wgpu::RenderPipelineDescriptor {
            label: Some(label),
            layout: Some(&pipeline_layout),
            vertex: wgpu::VertexState {
                module: shader,
                entry_point: Some("vs_main"),
                compilation_options: Default::default(),
                buffers: &[wgpu::VertexBufferLayout {
                    array_stride: 12,
                    step_mode: wgpu::VertexStepMode::Vertex,
                    attributes: &[wgpu::VertexAttribute {
                        format: wgpu::VertexFormat::Float32x3,
                        offset: 0,
                        shader_location: 0,
                    }],
                }],
            },
            fragment: Some(wgpu::FragmentState {
                module: shader,
                entry_point: Some("fs_main"),
                compilation_options: Default::default(),
                targets: &[Some(wgpu::ColorTargetState {
                    format: color_format,
                    blend: Some(blend),
                    write_mask: wgpu::ColorWrites::ALL,
                })],
            }),
            primitive: wgpu::PrimitiveState {
                topology: wgpu::PrimitiveTopology::TriangleList,
                front_face: wgpu::FrontFace::Ccw,
                cull_mode: None,
                ..Default::default()
            },
            depth_stencil: Some(wgpu::DepthStencilState {
                format: depth_format,
                depth_write_enabled: Some(depth_write),
                depth_compare: Some(depth_compare),
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
            instance_size,
            indirect_offset,
        }
    }

    /// (重)建 atlas 绑定:直接复用调用方传入的方块 atlas 视图与采样器,
    /// 与 terrain/water/lod/crack 同一图集。atlas 每次上传都整体替换,bind
    /// 随之重建;调用方是 `OffscreenRenderer::upload_atlas` 末尾。
    pub fn rebuild_bind(
        &mut self,
        device: &wgpu::Device,
        atlas_view: &wgpu::TextureView,
        sampler: &wgpu::Sampler,
    ) {
        self.bind = Some(device.create_bind_group(&wgpu::BindGroupDescriptor {
            label: Some("entity resources"),
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
                        size: Some((self.instance_size as u64).try_into().unwrap()),
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
        }));
    }

    /// 校验 instance 段字节:96 的倍数且不超过容量。
    pub fn instances_valid(&self, instances: &[u8]) -> bool {
        instances.len().is_multiple_of(ENTITY_INSTANCE_BYTES)
            && instances.len() <= self.instance_size
    }

    /// 上传 camera(80B,布局镜像 Go `encodeAvatarCameraInto`:viewproj +
    /// daylight + 零填充)与 instances,写 indirect 参数。
    pub fn upload(
        &self,
        queue: &wgpu::Queue,
        view_proj: &[f32; 16],
        daylight: f32,
        instances: &[u8],
    ) {
        let mut camera = [0u8; CAMERA_BYTES];
        for (i, v) in view_proj.iter().enumerate() {
            camera[i * 4..i * 4 + 4].copy_from_slice(&v.to_le_bytes());
        }
        camera[64..68].copy_from_slice(&daylight.to_le_bytes());
        queue.write_buffer(&self.dynamic, 0, &camera);
        queue.write_buffer(&self.dynamic, INSTANCE_OFFSET as u64, instances);
        let count = (instances.len() / ENTITY_INSTANCE_BYTES) as u32;
        let mut indirect = [0u8; INDIRECT_BYTES];
        indirect[0..4].copy_from_slice(&CUBE_INDEX_COUNT.to_le_bytes());
        indirect[4..8].copy_from_slice(&count.to_le_bytes());
        queue.write_buffer(&self.dynamic, self.indirect_offset as u64, &indirect);
    }

    /// 在已有颜色/深度附件之上录制本 pass(LoadOp::Load,镜像 Go
    /// `LoadClear: false`)。atlas 未上传时直接返回、不开始 render pass——
    /// 与 `terrain_bind == None` 及裂纹 pass 同语义:预热后恒有 atlas。
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
mod layout_tests {
    use super::*;

    /// 跨语言实例布局锁:96 字节 = mat4(64)+ color(16)+ 材质 u32(4)+
    /// 保留零填充(12);容量 75 具 × 6 部件 = 450 实例不变;哨兵与有效层号
    /// 不相交。与 Go `avatarInstanceBytes` 及桥测试同值,漂移即红。
    #[test]
    fn instance_layout_matches_go_bridge() {
        assert_eq!(ENTITY_INSTANCE_BYTES, 96);
        assert_eq!(AVATAR_MAX_INSTANCES, 450);
        assert_eq!(AVATAR_MATERIAL_SOLID, u32::MAX);
        assert_eq!(AVATAR_MATERIAL_OFFSET, 80);
        assert_eq!(
            AVATAR_MAX_INSTANCES * ENTITY_INSTANCE_BYTES,
            43200,
            "450 实例的字节数"
        );
    }

    /// 材质槽编解码锁:偏移 80 处小端 u32,保留区 84..96 全零;哨兵
    /// 与示例有效层可辨(具体牛层号由 Go 素材包守卫,此处只锁不相交性)。
    #[test]
    fn material_slot_codec_roundtrip() {
        let mut instance = [0u8; ENTITY_INSTANCE_BYTES];
        instance[AVATAR_MATERIAL_OFFSET..AVATAR_MATERIAL_OFFSET + 4]
            .copy_from_slice(&AVATAR_MATERIAL_SOLID.to_le_bytes());
        assert_eq!(
            u32::from_le_bytes(
                instance[AVATAR_MATERIAL_OFFSET..AVATAR_MATERIAL_OFFSET + 4]
                    .try_into()
                    .unwrap()
            ),
            AVATAR_MATERIAL_SOLID
        );
        assert!(instance[84..96].iter().all(|&b| b == 0), "保留区必须全零");
        let example_layer: u32 = 0;
        assert_ne!(
            example_layer, AVATAR_MATERIAL_SOLID,
            "示例层号不得与哨兵重合"
        );
    }
}
