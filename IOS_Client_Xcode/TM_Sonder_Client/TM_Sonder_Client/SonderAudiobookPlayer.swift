import AVFoundation
import Combine
import MediaPlayer
import SonderAPI
import SwiftUI

/// Owned by the library model, so dismissing a detail sheet never tears down audio.
@MainActor
final class SonderAudiobookPlayer: ObservableObject {
    @Published private(set) var item: SonderMediaItem?
    @Published private(set) var seconds = 0.0
    @Published private(set) var duration = 0.0
    @Published private(set) var isPlaying = false
    @Published private(set) var isLoading = false
    @Published private(set) var message: String?
    @Published private(set) var chapters: [SonderAudiobookChapter] = []
    @Published private(set) var sleepDeadline: Date?
    private var sleepTask: Task<Void, Never>?
    @Published var rate: Float = 1 { didSet { if isPlaying { player.rate = rate }; updateNowPlaying() } }
    private let player = AVPlayer()
    private weak var model: SonderClientModel?
    private var serverURL: URL?
    private var parts: [SonderMediaItem] = []
    private var partIndex = 0
    private var generation = UUID()
    private var observers: [NSObjectProtocol] = []
    private var commands: [(MPRemoteCommand, Any)] = []
    private var timeObserver: Any?
    private var writeTask: Task<Void, Never>?
    private var lastCheckpoint = Date.distantPast
    private var resumeAfterInterruption = false

    init() {
        timeObserver = player.addPeriodicTimeObserver(forInterval: CMTime(seconds: 1, preferredTimescale: 600), queue: .main) { [weak self] time in
            let position = time.seconds
            Task { @MainActor [weak self] in
                guard let self, self.item != nil, position.isFinite else { return }
                if let deadline = self.sleepDeadline, Date() >= deadline { self.pause(); self.setSleep(minutes: nil) }
                self.seconds = max(position, 0)
                if let length = self.player.currentItem?.duration.seconds, length.isFinite, length > 0 { self.duration = length }
                self.isPlaying = self.player.rate > 0
                if self.player.currentItem?.status == .failed {
                    self.message = self.player.currentItem?.error?.localizedDescription ?? "Audio stopped. Tap Play to reconnect."
                }
                self.updateNowPlaying()
                if Date().timeIntervalSince(self.lastCheckpoint) >= 10 { self.checkpoint() }
            }
        }
        let center = NotificationCenter.default
        observers.append(center.addObserver(forName: AVPlayerItem.didPlayToEndTimeNotification, object: nil, queue: .main) { [weak self] notification in
            let finished = notification.object as? AVPlayerItem
            Task { @MainActor [weak self] in
                guard let self, finished === self.player.currentItem else { return }
                self.checkpoint()
                if self.partIndex + 1 < self.parts.count {
                    self.partIndex += 1
                    await self.loadPart(resume: false)
                } else { self.pause() }
            }
        })
        observers.append(center.addObserver(forName: AVAudioSession.didBecomeInactiveNotification, object: nil, queue: .main) { [weak self] notification in
            let context = notification.userInfo?[AVAudioSession.deactivationContextKey] as? AVAudioSession.DeactivationContext
            guard context?.source == .system else { return }
            Task { @MainActor [weak self] in
                guard let self else { return }
                self.resumeAfterInterruption = self.isPlaying
                self.pause(clearInterruption: false)
            }
        })
        observers.append(center.addObserver(forName: AVAudioSession.resumptionRecommendationNotification, object: nil, queue: .main) { [weak self] notification in
            let context = notification.userInfo?[AVAudioSession.resumptionContextKey] as? AVAudioSession.ResumptionContext
            let shouldResume = context?.recommendation == .shouldResume
            Task { @MainActor [weak self] in
                guard let self else { return }
                let resume = self.resumeAfterInterruption && shouldResume
                self.resumeAfterInterruption = false
                if resume { self.play() }
            }
        })
        observers.append(center.addObserver(forName: AVAudioSession.routeChangeNotification, object: nil, queue: .main) { [weak self] notification in
            let reason = notification.userInfo?[AVAudioSessionRouteChangeReasonKey] as? UInt
            Task { @MainActor [weak self] in
                if reason == AVAudioSession.RouteChangeReason.oldDeviceUnavailable.rawValue { self?.pause() }
            }
        })
        observers.append(center.addObserver(forName: UIApplication.didEnterBackgroundNotification, object: nil, queue: .main) { [weak self] _ in
            Task { @MainActor [weak self] in self?.checkpoint() }
        })
        registerRemoteCommands()
    }

    func start(_ selected: SonderMediaItem, model: SonderClientModel) async {
        if (item?.id == selected.id || (selected.bookGroupID != nil && item?.bookGroupID == selected.bookGroupID)), serverURL == model.serverBaseURL { play(); return }
        stop()
        self.model = model
        serverURL = model.serverBaseURL
        let grouped = selected.bookGroupID.map { group in model.items.filter { $0.kind == .audiobook && $0.bookGroupID == group } } ?? []
        parts = grouped.isEmpty ? [selected] : grouped.sorted { ($0.bookPartIndex ?? 0) < ($1.bookPartIndex ?? 0) }
        let unfinished = parts.indices.filter { index in
            let part = parts[index]
            let progress = model.progressByItemID[part.id]
            let length = progress?.duration ?? part.durationSeconds
            return length <= 0 || (progress?.seconds ?? part.progressSeconds) / length < 0.96
        }
        partIndex = unfinished.filter { (model.progressByItemID[parts[$0].id]?.seconds ?? parts[$0].progressSeconds) > 5 }
            .max { (model.progressByItemID[parts[$0].id]?.updatedAt ?? .distantPast) < (model.progressByItemID[parts[$1].id]?.updatedAt ?? .distantPast) }
            ?? unfinished.first ?? 0
        await loadPart(resume: true)
    }

    private func loadPart(resume: Bool) async {
        guard let model, parts.indices.contains(partIndex), serverURL == model.serverBaseURL else { return }
        generation = UUID()
        let request = generation
        let next = parts[partIndex]
        player.pause()
        player.replaceCurrentItem(with: nil)
        item = next
        isPlaying = false
        isLoading = true
        message = nil
        seconds = 0
        duration = next.durationSeconds
        chapters = []
        do {
            let saved = model.progressByItemID[next.id]?.seconds ?? next.progressSeconds
            let response = try await model.startPlayback(for: next, seconds: resume ? saved : 0, duration: next.durationSeconds)
            guard generation == request, serverURL == model.serverBaseURL else { return }
            let url = model.localMediaURL(for: next) ?? model.resolvedServerURL(from: response.streamURL) ?? model.streamURL(for: next)
            let asset = url.isFileURL ? AVURLAsset(url: url) : model.streamAsset(for: url)
            player.replaceCurrentItem(with: AVPlayerItem(asset: asset))
            duration = response.duration > 0 ? response.duration : next.durationSeconds
            let offset = resume ? response.seconds : 0
            if offset > 0 { await player.seek(to: CMTime(seconds: offset, preferredTimescale: 600), toleranceBefore: .zero, toleranceAfter: .zero) }
            guard generation == request, serverURL == model.serverBaseURL else { return }
            seconds = offset
            isLoading = false
            lastCheckpoint = Date()
            play()
            let timeline = (try? await model.audiobookChapters(for: next)) ?? []
            guard generation == request, serverURL == model.serverBaseURL else { return }
            let bookOffset = parts.prefix(partIndex).reduce(0) { $0 + $1.durationSeconds }
            chapters = timeline.filter { $0.partID == next.id || ($0.partID == nil && partIndex == 0) }.map {
                SonderAudiobookChapter(index: $0.index, title: $0.title, startSeconds: max(0, $0.startSeconds - bookOffset), endSeconds: $0.endSeconds.map { max(0, $0 - bookOffset) })
            }
        } catch {
            guard generation == request else { return }
            isLoading = false
            message = error.localizedDescription
        }
    }

    func play() {
        guard item != nil else { return }
        if let deadline = sleepDeadline, Date() >= deadline { setSleep(minutes: nil); return }
        if player.currentItem == nil || player.currentItem?.status == .failed {
            Task { await loadPart(resume: true) }
            return
        }
        do {
            try AVAudioSession.sharedInstance().setCategory(.playback, mode: .spokenAudio, policy: .longFormAudio)
            try AVAudioSession.sharedInstance().setActive(true)
            player.playImmediately(atRate: rate)
            isPlaying = true
            message = nil
            updateNowPlaying()
        } catch { message = error.localizedDescription }
    }

    func pause(clearInterruption: Bool = true) {
        if clearInterruption { resumeAfterInterruption = false }
        player.pause()
        isPlaying = false
        checkpoint()
        updateNowPlaying()
    }

    func seek(_ position: Double) {
        guard position.isFinite, duration > 0 else { return }
        seconds = min(max(position, 0), duration)
        player.seek(to: CMTime(seconds: seconds, preferredTimescale: 600), toleranceBefore: .zero, toleranceAfter: .zero)
        checkpoint()
        updateNowPlaying()
    }

    func stop() {
        setSleep(minutes: nil)
        checkpoint()
        generation = UUID()
        player.pause()
        player.replaceCurrentItem(with: nil)
        item = nil
        isPlaying = false
        isLoading = false
        parts = []
        MPNowPlayingInfoCenter.default().nowPlayingInfo = nil
        try? AVAudioSession.sharedInstance().setActive(false, options: .notifyOthersOnDeactivation)
    }

    func setSleep(minutes: Int?) {
        sleepTask?.cancel()
        sleepDeadline = minutes.map { Date().addingTimeInterval(Double($0 * 60)) }
        guard let minutes else { return }
        sleepTask = Task { [weak self] in
            do { try await Task.sleep(for: .seconds(minutes * 60)) } catch { return }
            self?.pause()
            self?.sleepDeadline = nil
        }
    }

    func checkpoint() {
        guard let item, let model, duration.isFinite, duration > 0, serverURL == model.serverBaseURL, !isLoading else { return }
        lastCheckpoint = Date()
        let server = serverURL
        model.stagePlaybackCheckpoint(for: item, seconds: seconds, duration: duration)
        let previous = writeTask
        writeTask = Task { [weak model] in
            await previous?.value
            guard let model, model.serverBaseURL == server else { return }
            await model.flushPendingProgress()
        }
    }

    private func updateNowPlaying() {
        guard let item else { return }
        MPNowPlayingInfoCenter.default().nowPlayingInfo = [
            MPMediaItemPropertyTitle: item.title,
            MPMediaItemPropertyArtist: item.author ?? "Audiobook",
            MPMediaItemPropertyPlaybackDuration: duration,
            MPNowPlayingInfoPropertyElapsedPlaybackTime: seconds,
            MPNowPlayingInfoPropertyPlaybackRate: isPlaying ? rate : 0,
            MPNowPlayingInfoPropertyDefaultPlaybackRate: rate
        ]
    }

    private func registerRemoteCommands() {
        let center = MPRemoteCommandCenter.shared()
        for (command, action) in [(center.playCommand, 0), (center.pauseCommand, 1), (center.togglePlayPauseCommand, 2)] {
            let target = command.addTarget { [weak self] _ in
                Task { @MainActor [weak self] in
                    guard let self else { return }
                    if action == 0 { self.play() } else if action == 1 { self.pause() } else { self.isPlaying ? self.pause() : self.play() }
                }
                return .success
            }
            commands.append((command, target))
        }
        for (command, delta) in [(center.skipForwardCommand, 30.0), (center.skipBackwardCommand, -30.0)] {
            command.preferredIntervals = [30]
            let target = command.addTarget { [weak self] _ in
                Task { @MainActor [weak self] in guard let self else { return }; self.seek(self.seconds + delta) }
                return .success
            }
            commands.append((command, target))
        }
        let target = center.changePlaybackPositionCommand.addTarget { [weak self] event in
            guard let event = event as? MPChangePlaybackPositionCommandEvent else { return .commandFailed }
            let position = event.positionTime
            Task { @MainActor [weak self] in self?.seek(position) }
            return .success
        }
        commands.append((center.changePlaybackPositionCommand, target))
    }
}

struct SonderAudiobookMiniPlayer: View {
    @ObservedObject var audio: SonderAudiobookPlayer
    @ObservedObject var model: SonderClientModel
    @State private var showingPlayer = false
    var body: some View {
        if let item = audio.item {
            HStack {
                Button { showingPlayer = true } label: { VStack(alignment: .leading) {
                    Text(item.title).font(.subheadline.weight(.semibold)).lineLimit(1)
                    Text(audio.message ?? "Listening continues while you browse").font(.caption).lineLimit(2)
                }.foregroundStyle(.primary) }.buttonStyle(.plain).accessibilityLabel("Open audiobook player")
                Spacer()
                Button { audio.isPlaying ? audio.pause() : audio.play() } label: { Image(systemName: audio.isPlaying ? "pause.fill" : "play.fill").frame(width: 44, height: 44) }
                    .accessibilityLabel(audio.isPlaying ? "Pause audiobook" : "Play audiobook")
                Button { audio.stop() } label: { Image(systemName: "xmark").frame(width: 44, height: 44) }.accessibilityLabel("Stop audiobook")
            }.padding(.horizontal, 16).padding(.vertical, 8).background(.regularMaterial)
                .sheet(isPresented: $showingPlayer) { SonderAudiobookPlaybackScreen(model: model, audio: audio, item: item) }
        }
    }
}

struct SonderAudiobookPlaybackScreen: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var model: SonderClientModel
    @ObservedObject var audio: SonderAudiobookPlayer
    let item: SonderMediaItem
    @State private var showingChapters = false
    var body: some View {
        NavigationStack {
            VStack(spacing: 24) {
                Image(systemName: "headphones").font(.system(size: 64)).foregroundStyle(SonderPalette.ironGrey)
                Text(audio.item?.title ?? item.title).font(.title2.bold()).multilineTextAlignment(.center)
                if audio.isLoading { ProgressView("Preparing audio") }
                if let message = audio.message { Text(message).foregroundStyle(.secondary) }
                Slider(value: Binding(get: { audio.seconds }, set: { audio.seek($0) }), in: 0...max(audio.duration, 1)).accessibilityLabel("Audio position")
                Text("\(Int(audio.seconds) / 60) min of \(Int(audio.duration) / 60) min").font(.caption.monospacedDigit())
                HStack(spacing: 30) {
                    Button { audio.seek(audio.seconds - 30) } label: { Image(systemName: "gobackward.30").font(.title).frame(minWidth: 44, minHeight: 44) }.accessibilityLabel("Back 30 seconds")
                    Button { audio.isPlaying ? audio.pause() : audio.play() } label: { Image(systemName: audio.isPlaying ? "pause.circle.fill" : "play.circle.fill").font(.system(size: 64)) }.accessibilityLabel(audio.isPlaying ? "Pause" : "Play")
                    Button { audio.seek(audio.seconds + 30) } label: { Image(systemName: "goforward.30").font(.title).frame(minWidth: 44, minHeight: 44) }.accessibilityLabel("Forward 30 seconds")
                }
                Picker("Playback speed", selection: $audio.rate) { ForEach([Float(0.75), 1, 1.25, 1.5, 1.75, 2], id: \.self) { Text("\($0, specifier: "%.2g")×").tag($0) } }.pickerStyle(.menu)
                HStack {
                    Button("Chapters") { showingChapters = true }.disabled(audio.chapters.isEmpty)
                    Menu("Sleep timer") {
                        ForEach([15, 30, 45, 60], id: \.self) { minutes in Button("\(minutes) minutes") { audio.setSleep(minutes: minutes) } }
                        Button("Turn off") { audio.setSleep(minutes: nil) }
                    }
                }
                if let deadline = audio.sleepDeadline { Text("Stops at \(deadline, style: .time)").font(.caption) }
                Spacer()
            }.padding(24).navigationTitle("Listening").toolbar { Button("Done") { audio.checkpoint(); dismiss() } }
        }.task {
            let sameBook = item.bookGroupID != nil && audio.item?.bookGroupID == item.bookGroupID
            if audio.item?.id != item.id && !sameBook { await audio.start(item, model: model) }
        }
        .sheet(isPresented: $showingChapters) {
            NavigationStack {
                List(audio.chapters) { chapter in Button(chapter.title) { audio.seek(chapter.startSeconds); audio.play(); showingChapters = false } }
                    .navigationTitle("Chapters in this part").toolbar { Button("Done") { showingChapters = false } }
            }
        }
    }
}
