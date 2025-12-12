# Orvalho
(dew, like rain but not rain, not from clouds)

The objective of this is a system that can potentially run anywhere and take a set of target independent payloads to run as
actors, in actor model meaning.

Imagine using cheap devices that are often throwed out just because they can't run the latest apps and with limited utility
because they can't run a full featured GNU/Linux distro, but suck very little energy
compared to an older PC!

## Objectives
- Allow older, potentially unsupported devices, to serve selfhosting needs for their owners. Multi tenant is out of scope and
we can assume the user isn't a threat to its own device. It should work in the stock rom, which has the drivers set up already.

## Actors
- Must be either a JavaScript bundle or a WASM bundle.
- Run isolated, but necessarily in the same process.
  - Most legacy systems don't have very trustworthy isolation mechanisms, but V8 has!
- Don't share mutable state with other actors.
- React to events
  - Actor for a web server: reacts to the fetch event and respond with a Response, like a Service Worker.
  - Actor for a generic worker: takes arbitrary events and can respond back to those events.
  - Actor that interacts with a database: essentially exposes a repository pattern.
- Platform expose native actors in the bus if the actor is authorized, using [preferentially standardized web APIs](https://wintertc.org/).
  - Native actor that runs compute using something like WebGPU: setup inferences for specific models.
  - Native bridge actor: connects one actor to another actor running in other machines.
  - Native actor that interacts with specific devices like cameras, mics, USB, ...
- Each actor has a set of permissions that are accepted at install time.
- Actor bundles are distributed using some kind of package file where sets of actors are installed together, like an APK, a ZIP with a manifest, JS and WASM files.
- How to identify each actor?
  - UUIDs like dotnet GAC? UUIDv7 to hide ctime in the UUID?
  - That inverted domain thing Java uses
  - Simple slugs: potential collisions?
 
## Communication
- Wireguard
  - Full userspace based implementations.
  - Data plane is essentially solved
  - Control plane
    - Tailscale?
    - ED25519 public keys allow list?
    - Some kind of domain where each device gets their own virtual domain so it works like MagicDNS.
    - User shouldn't need to worry about IPs, only wether the phone is somehow connected to the Internet.
  - Maybe the underlying network stack can have some vulnerabilities that may make things difficult.
- How to setup the bus?
  - GRPC? Seems limited for this.
  - Some kind of contract based system would be preferred.
  - Is Wayland a good fit? Would be madness to setup?
    - XML specs -> codegen -> fill the gaps
    - Probably C focused, Rust is preferred for that
## Packaging
- Single file all in one binaries
- Platform specific wrappers
  - APKs
  - Containers
  - VMs
  - Packages in general
  - UEFI unikernels?

## Tech
- Either Go or Rust
  - Rust has more stuff done around V8 and isolates: Deno
  - Go has V8 bindings but don't have the primitives to setup the native features
    - And it's less cool when CGO is involved
   
## Auth
- User installs the manager
- Manager creates a ED25519 key
- User installs the system in a worker phone
- User pairs the worker phone with the manager
- User starts submiting or accessing stuff for the phone anywhere he goes as Wireguard and friends do it's thing (UPNP, STUN/TURN, DERP) to rendezvous with the phone.

## Scalability
- Not a issue. One person will very unlikely have more than three phones to setup as workers.
- Also, the idea is that people would use their own devices for their own uses, so multi tenant is not a worry.

## TODO
- [ ] Make this more exaustive
- [ ] Experiment with the possibilities
- [ ] Make a implementation plan for a prototype
- [ ] Gather feedback from people that know more than me, and the target audience of this.

   
    
