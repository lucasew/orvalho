# Mesh Actor Runtime - Breakdown de Componentes

## 1. Actor Runtime Core

### 1.1 Actor Lifecycle Manager
**Responsabilidade:** Spawn, shutdown, resource cleanup

**Tarefas:**
- [ ] Struct `Actor` com runtime/context/capabilities
- [ ] `SpawnActor(bundle, caps)` - cria novo ator
- [ ] `Shutdown(ctx, graceful)` - cleanup recursos
- [ ] Memory limit enforcement
- [ ] Timeout/interrupt handling

**Dependências:** modernc/quickjs

---

### 1.2 Event Loop
**Responsabilidade:** Processar eventos assíncronos

**Tarefas:**
- [ ] Channel de eventos externos (`chan Event`)
- [ ] Timer heap (setTimeout/setInterval)
- [ ] Loop principal (select events/timers/microtasks)
- [ ] `ExecutePendingJob()` integration
- [ ] Graceful shutdown do loop

**Dependências:** 1.1

---

### 1.3 Bundle Loader
**Responsabilidade:** Carregar e validar código JS/Wasm

**Tarefas:**
- [ ] Parse bundle (JS ou Wasm)
- [ ] Validate manifest (capabilities declaradas)
- [ ] Load no context QuickJS
- [ ] Export detection (`default.fetch`)
- [ ] Error handling (syntax, runtime)

**Dependências:** 1.1

---

## 2. Capability System

### 2.1 Capability Definition
**Responsabilidade:** Modelar permissões

**Tarefas:**
- [ ] Struct `CapabilitySet` (GPU, Camera, Actors, etc)
- [ ] Parser de manifest JSON
- [ ] Validation rules (limites válidos)
- [ ] Serialização/deserialização

**Dependências:** nenhuma

---

### 2.2 Env Injection
**Responsabilidade:** Injetar bindings no contexto JS

**Tarefas:**
- [ ] `injectEnv(context, caps)` - cria objeto env
- [ ] Factory de bindings condicionais
- [ ] Global scope setup (fetch, setTimeout, etc)
- [ ] Error boundary por binding

**Dependências:** 2.1, 1.1

---

## 3. Web APIs Standard

### 3.1 Fetch API
**Responsabilidade:** HTTP client padrão

**Tarefas:**
- [ ] `env.fetch()` ou global `fetch()`
- [ ] Request/Response objects (serialização Go↔JS)
- [ ] Headers, Body streams
- [ ] Timeout, abort signal
- [ ] HTTPS via Go http.Client

**Dependências:** 1.2 (async), 2.2

---

### 3.2 Timers
**Responsabilidade:** setTimeout, setInterval, clearTimeout

**Tarefas:**
- [ ] Global `setTimeout(callback, delay)`
- [ ] Global `setInterval(callback, delay)`
- [ ] Timer heap (min-heap por deadline)
- [ ] Clear functions
- [ ] Integration com event loop

**Dependências:** 1.2

---

### 3.3 Crypto/Encoding
**Responsabilidade:** APIs Web standard auxiliares

**Tarefas:**
- [ ] `TextEncoder`/`TextDecoder`
- [ ] `crypto.randomUUID()`
- [ ] `crypto.subtle` básico (se necessário)
- [ ] Base64 encode/decode

**Dependências:** 2.2

---

## 4. GPU Capability (via purego)

### 4.1 Purego Setup
**Responsabilidade:** Load wgpu-native dinamicamente

**Tarefas:**
- [ ] `purego.Dlopen("libwgpu_native.so")`
- [ ] Symbol loading (wgpuInstanceCreate, etc)
- [ ] Error handling (lib not found)
- [ ] Multi-platform paths (Linux/macOS/Windows)

**Dependências:** nenhuma

---

### 4.2 WGPU Bindings Core
**Responsabilidade:** Structs e funções C básicas

**Tarefas:**
- [ ] Struct layouts C (WGPUInstance, WGPUDevice, etc)
- [ ] Function signatures via purego
- [ ] Instance creation
- [ ] Adapter/Device selection
- [ ] Error handling wgpu

**Dependências:** 4.1

---

### 4.3 Buffer Management
**Responsabilidade:** Criar/gerenciar GPU buffers

**Tarefas:**
- [ ] `createBuffer(size, usage)` 
- [ ] Write data (CPU → GPU)
- [ ] Read data (GPU → CPU)
- [ ] Buffer cleanup/destroy
- [ ] Pool de buffers (otimização)

**Dependências:** 4.2

---

### 4.4 Shader Compilation
**Responsabilidade:** WGSL → executável

**Tarefas:**
- [ ] Parse WGSL source
- [ ] Compile shader module
- [ ] Create compute pipeline
- [ ] Shader cache (optional)
- [ ] Error reporting

**Dependências:** 4.2

---

### 4.5 Compute Dispatch
**Responsabilidade:** Executar compute shaders

**Tarefas:**
- [ ] Bind group setup (buffers → shader slots)
- [ ] `dispatch(workgroups)` 
- [ ] Command encoder/queue
- [ ] Async completion (channel)
- [ ] Result retrieval

**Dependências:** 4.3, 4.4

---

### 4.6 GPU Binding JS API
**Responsabilidade:** Expor GPU pro ator via env.GPU

**Tarefas:**
- [ ] `env.GPU.dispatch(params)` → Promise
- [ ] `env.GPU.createBuffer(size)` → BufferId
- [ ] Serialização params JS↔Go
- [ ] Integration com event loop
- [ ] Error handling pra JS

**Dependências:** 4.5, 2.2, 1.2

---

## 5. Camera Capability (via purego)

### 5.1 V4L2 Bindings (Linux)
**Responsabilidade:** Acesso câmera Linux

**Tarefas:**
- [ ] Open `/dev/video*` via syscall
- [ ] Query capabilities (formats, resolutions)
- [ ] Set format (YUYV, MJPEG, etc)
- [ ] Request buffers (mmap)
- [ ] Start/stop streaming

**Dependências:** 4.1 pattern (purego/syscall)

---

### 5.2 Platform Abstractions
**Responsabilidade:** Unificar APIs de câmera

**Tarefas:**
- [ ] Interface `CameraBackend`
- [ ] Linux: v4l2 impl
- [ ] macOS: AVFoundation via purego (future)
- [ ] Windows: DirectShow via purego (future)
- [ ] Fallback/mock

**Dependências:** 5.1

---

### 5.3 Frame Streaming
**Responsabilidade:** Delivery de frames pro ator

**Tarefas:**
- [ ] Goroutine de capture loop
- [ ] Channel de frames (bounded)
- [ ] Format conversion (YUV→RGB se necessário)
- [ ] Backpressure handling
- [ ] Stop gracefully

**Dependências:** 5.2

---

### 5.4 Camera Binding JS API
**Responsabilidade:** Expor camera pro ator via env.CAMERA

**Tarefas:**
- [ ] `env.CAMERA.capture(constraints)` → Promise<StreamId>
- [ ] `env.CAMERA.getFrame(streamId)` → Promise<Frame>
- [ ] `env.CAMERA.stop(streamId)`
- [ ] Frame como ArrayBuffer/Blob
- [ ] Integration event loop

**Dependências:** 5.3, 2.2, 1.2

---

## 6. Actor Communication

### 6.1 Local Actor Registry
**Responsabilidade:** Tracking de atores locais

**Tarefas:**
- [ ] Map `actorId → *Actor`
- [ ] Register/unregister
- [ ] Lookup by ID
- [ ] Lifecycle hooks (on spawn/shutdown)
- [ ] Thread-safe access

**Dependências:** 1.1

---

### 6.2 Message Passing Local
**Responsabilidade:** Mensagens entre atores mesmo node

**Tarefas:**
- [ ] `SendMessage(targetId, payload)`
- [ ] Serialização payload (JSON/msgpack)
- [ ] Delivery via event loop target
- [ ] Error handling (ator não existe)
- [ ] Timeout/retry

**Dependências:** 6.1, 1.2

---

### 6.3 Actor Stub JS API
**Responsabilidade:** Interface JS pra comunicação

**Tarefas:**
- [ ] `env.ACTORS.get(id)` → ActorStub
- [ ] `stub.send(message)` → Promise
- [ ] Request/response pattern (optional)
- [ ] Error propagation pra JS

**Dependências:** 6.2, 2.2

---

## 7. Mesh Network

### 7.1 Wireguard Setup
**Responsabilidade:** VPN overlay entre nodes

**Tarefas:**
- [ ] wireguard-go integration
- [ ] Config generation (keys, peers)
- [ ] Interface up/down
- [ ] Route setup
- [ ] Health check (ping peers)

**Dependências:** nenhuma (lib externa)

---

### 7.2 Magicsock Integration
**Responsabilidade:** NAT traversal

**Tarefas:**
- [ ] STUN/DERP client
- [ ] Hole punching
- [ ] Relay fallback
- [ ] Connection upgrade (relay→direct)
- [ ] keepalive

**Dependências:** 7.1

---

### 7.3 Node Discovery
**Responsabilidade:** Descobrir peers na mesh

**Tarefas:**
- [ ] Bootstrap node connection
- [ ] Peer list propagation (gossip)
- [ ] Capability advertisement (GPU, camera)
- [ ] Heartbeat protocol
- [ ] Failure detection

**Dependências:** 7.2

---

### 7.4 Actor Routing
**Responsabilidade:** Localizar ator na mesh

**Tarefas:**
- [ ] DHT ou registry distribuído
- [ ] `actorId → nodeId` mapping
- [ ] Cache de rotas
- [ ] Update on migration
- [ ] Query protocol

**Dependências:** 7.3

---

### 7.5 Remote Message Passing
**Responsabilidade:** Mensagens cross-node

**Tarefas:**
- [ ] Lookup ator (local registry → remote routing)
- [ ] Serialização wire protocol
- [ ] Send via mesh overlay
- [ ] Delivery confirmation/retry
- [ ] Fallback on failure

**Dependências:** 7.4, 6.2

---

## 8. Wasm Runtime (Opcional/Futuro)

### 8.1 Wasmtime Integration
**Tarefas:**
- [ ] wasmtime-go setup
- [ ] Component model support
- [ ] Module instantiation
- [ ] Memory/resource limits

**Dependências:** 1.1 pattern

---

### 8.2 WIT Interface Definition
**Tarefas:**
- [ ] Define WIT para GPU, Camera, Actors
- [ ] Bindgen pra Rust/TinyGo
- [ ] Host implementation
- [ ] Guest stubs

**Dependências:** 8.1, 4.6, 5.4, 6.3

---

## 9. Observability

### 9.1 Logging
**Tarefas:**
- [ ] Structured logging (slog)
- [ ] Per-actor log context
- [ ] Log levels
- [ ] Rotation/export

**Dependências:** nenhuma

---

### 9.2 Metrics
**Tarefas:**
- [ ] Actor count, memory usage
- [ ] Event loop latency
- [ ] GPU utilization
- [ ] Message throughput
- [ ] Prometheus export

**Dependências:** todos componentes

---

### 9.3 Tracing
**Tarefas:**
- [ ] OpenTelemetry integration
- [ ] Span per actor execution
- [ ] Distributed tracing (mesh)
- [ ] Sampling strategy

**Dependências:** 7.5

---

## Ordem de Implementação Sugerida

**Milestone 1: Runtime Básico (Local)**
1. Actor Lifecycle (1.1)
2. Event Loop (1.2)
3. Bundle Loader (1.3)
4. Capability Definition (2.1)
5. Env Injection (2.2)
6. Timers (3.2)
7. Fetch API (3.1)

**Milestone 2: GPU Local**
1. Purego Setup (4.1)
2. WGPU Core (4.2)
3. Buffer Management (4.3)
4. Shader Compilation (4.4)
5. Compute Dispatch (4.5)
6. GPU JS API (4.6)

**Milestone 3: Actor Communication Local**
1. Actor Registry (6.1)
2. Message Passing (6.2)
3. Actor Stub API (6.3)

**Milestone 4: Mesh Distribution**
1. Wireguard (7.1)
2. Magicsock (7.2)
3. Node Discovery (7.3)
4. Actor Routing (7.4)
5. Remote Messaging (7.5)

**Milestone 5: Camera + Observability**
1. V4L2 Bindings (5.1-5.4)
2. Logging/Metrics (9.1-9.2)
