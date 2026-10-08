import AVKit
import SwiftUI
import UIKit
import SonderAPI

struct MediaDetailView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var model: SonderClientModel
    let item: SonderMediaItem
    @State private var isShowingPlayer = false
    @State private var isShowingReader = false
    @State private var playbackMessage: String?
    @State private var confirmRemoval = false
    @State private var seconds: Double
    @State private var duration: Double

    init(model: SonderClientModel, item: SonderMediaItem) {
        self.model = model
        self.item = item
        let progress = model.progress(for: item)
        _seconds = State(initialValue: progress.seconds)
        _duration = State(initialValue: progress.duration > 0 ? progress.duration : item.durationSeconds)
    }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 18) {
                HStack(alignment: .top, spacing: 16) {
                    PosterPlaceholder(item: item, artworkURL: model.artworkURL(for: item), accessToken: model.accessToken)
                        .frame(width: 112, height: 164)

                    VStack(alignment: .leading, spacing: 8) {
                        Text(item.title)
                            .font(.title2.weight(.semibold))
                            .foregroundStyle(SonderPalette.text)
                            .fixedSize(horizontal: false, vertical: true)
                        Text(item.subtitle)
                            .font(.subheadline)
                            .foregroundStyle(SonderPalette.textLight)
                        Label(item.kind.label, systemImage: iconName)
                            .foregroundStyle(SonderPalette.textLight)
                    }
                }

                if item.kind == .ebook {
                    ebookActions
                } else if model.supportsPreparedVideo(item) || item.format.isPlayableInAVPlayer {
                    Button {
                        isShowingPlayer = true
                    } label: {
                        Label(playActionTitle, systemImage: "play.fill")
                            .font(.headline)
                            .frame(maxWidth: .infinity)
                            .padding(.vertical, 14)
                    }
                    .buttonStyle(.borderedProminent)
                    .tint(SonderPalette.ironGrey)

                    Text(model.isFullySavedOffline(item) ? "Saved on this device · Available offline" : "Streams from your server")
                        .font(.caption)
                        .foregroundStyle(SonderPalette.textLight)

                    if let playbackMessage {
                        Label(playbackMessage, systemImage: "exclamationmark.triangle")
                            .font(.caption)
                            .foregroundStyle(SonderPalette.ironGrey)
                            .padding(10)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .background(SonderPalette.surfaceDeep, in: RoundedRectangle(cornerRadius: 8))
                    }
                } else {
                    VStack(alignment: .leading, spacing: 10) {
                        Label("Not playable on iOS", systemImage: "play.slash")
                            .font(.headline)
                            .foregroundStyle(SonderPalette.text)
                        Text(item.format.clientPlaybackGuidance)
                            .font(.subheadline)
                            .foregroundStyle(SonderPalette.textLight)
                            .fixedSize(horizontal: false, vertical: true)
                        MetadataRow(label: "Format", value: item.format.rawValue.uppercased())
                        MetadataRow(label: "Player support", value: "Convert on Mac host")
                    }
                    .sonderPanel()
                }

                offlineActions

                if item.kind != .ebook { progressSection }
                if !item.summary.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                    DisclosureGroup("About this title") {
                        Text(item.summary)
                            .font(.body)
                            .foregroundStyle(SonderPalette.textLight)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .padding(.top, 8)
                    }
                    .sonderPanel()
                }
                DisclosureGroup("File details") { metadataSection.padding(.top, 8) }
                    .sonderPanel()
            }
            .padding()
        }
        .background(SonderPalette.background)
        .navigationTitle("Details")
        .navigationBarTitleDisplayMode(.inline)
        .confirmationDialog("Remove this download?", isPresented: $confirmRemoval, titleVisibility: .visible) {
            Button("Remove Download", role: .destructive) { model.removeOfflineDownload(for: item) }
        } message: {
            Text("The original stays on your server. You’ll need a connection to download it again.")
        }
        .toolbar {
            ToolbarItem(placement: .confirmationAction) {
                Button("Done") { dismiss() }
            }
        }
        .fullScreenCover(isPresented: $isShowingPlayer) {
            if item.kind == .audiobook {
                SonderAudiobookPlaybackScreen(model: model, audio: model.audiobookPlayer, item: item)
            } else {
            SonderPlaybackScreen(
                model: model,
                item: item,
                initialSeconds: seconds,
                initialDuration: duration
            ) { finalSeconds, finalDuration, message in
                seconds = finalSeconds
                duration = finalDuration
                playbackMessage = message
            }
            }
        }
        .sheet(isPresented: $isShowingReader) {
            if item.format == .epub {
                SonderEPUBReader(model: model, item: item)
            } else {
                SonderPDFReader(model: model, item: item)
            }
        }
    }

    @ViewBuilder
    private var ebookActions: some View {
        if item.format == .pdf {
            Button {
                isShowingReader = true
            } label: {
                Label("Read PDF", systemImage: "book.pages")
                    .font(.headline)
                    .frame(maxWidth: .infinity)
                    .padding(.vertical, 14)
            }
            .buttonStyle(.borderedProminent)
            .tint(SonderPalette.ironGrey)

            Text("Opens the original PDF from your Sonder server in the built-in reader.")
                .font(.caption)
                .foregroundStyle(SonderPalette.textLight)
        } else {
            VStack(alignment: .leading, spacing: 10) {
                Label("Read EPUB Offline", systemImage: "book.closed")
                    .font(.headline)
                    .foregroundStyle(SonderPalette.text)
                Text("Download this book, then read it here without a connection. Sonder remembers your section and reading position on this device.")
                    .font(.subheadline)
                    .foregroundStyle(SonderPalette.textLight)
                    .fixedSize(horizontal: false, vertical: true)
                MetadataRow(label: "Format", value: item.format.rawValue.uppercased())
                if model.localMediaURL(for: item) != nil {
                    Button { isShowingReader = true } label: {
                        Label("Read EPUB", systemImage: "book.pages")
                    }
                    .buttonStyle(.borderedProminent)
                }
            }
            .sonderPanel()
        }
    }

    @ViewBuilder
    private var offlineActions: some View {
        if model.supportsOfflineDownload(for: item) {
            VStack(alignment: .leading, spacing: 9) {
                Text("Offline access")
                    .font(.headline)
                    .foregroundStyle(SonderPalette.text)

                if let download = model.offlineDownloads[item.id] {
                    Label(
                        "\(model.isFullySavedOffline(item) ? "Saved on this iPhone" : (model.isVideo(item) ? "Offline copy incomplete" : "Part saved · book incomplete")) • \(ByteCountFormatter.string(fromByteCount: download.byteCount, countStyle: .file))",
                        systemImage: "checkmark.circle.fill"
                    )
                    .font(.caption)
                    .foregroundStyle(SonderPalette.ironGrey)
                    if !model.isFullySavedOffline(item) {
                        Button(model.isVideo(item) ? "Download again" : "Download remaining parts") { Task { await model.downloadForOffline(item) } }
                            .buttonStyle(.borderedProminent)
                    }
                    Button(role: .destructive) {
                        confirmRemoval = true
                    } label: {
                        Label("Remove Download", systemImage: "trash")
                    }
                    .buttonStyle(.bordered)
                } else if model.isDownloadingOffline(item) {
                    OfflineTransferStatus(progress: model.downloadProgress(for: item), isPaused: false)
                    if model.preparingOfflineItemIDs.contains(item.id) {
                        Text("Preparing a complete movie on the server. This continues if you cancel the download.")
                            .font(.caption).foregroundStyle(SonderPalette.textLight)
                    } else {
                        Button {
                            model.pauseOfflineDownload(for: item)
                        } label: {
                            Label("Pause Download", systemImage: "pause.fill")
                        }
                        .buttonStyle(.bordered)
                    }
                    if model.supportsPreparedVideo(item) {
                        Button("Cancel Download", role: .destructive) { model.cancelOfflineDownload(for: item) }
                            .buttonStyle(.bordered)
                    }
                } else if model.isOfflineDownloadPaused(item) {
                    OfflineTransferStatus(progress: model.downloadProgress(for: item), isPaused: true)
                    Button {
                        Task { await model.downloadForOffline(item) }
                    } label: {
                        Label("Resume Download", systemImage: "play.fill")
                    }
                    .buttonStyle(.borderedProminent)
                    .tint(SonderPalette.ironGrey)
                    if model.supportsPreparedVideo(item) {
                        Button("Cancel Download", role: .destructive) { model.cancelOfflineDownload(for: item) }
                            .buttonStyle(.bordered)
                    }
                } else {
                    Text("Download a complete copy for playback or reading away from your Sonder server.")
                        .font(.caption)
                        .foregroundStyle(SonderPalette.textLight)
                        .fixedSize(horizontal: false, vertical: true)
                    Button {
                        Task { await model.downloadForOffline(item) }
                    } label: {
                        Label("Download for Offline", systemImage: "arrow.down.circle")
                    }
                    .buttonStyle(.borderedProminent)
                    .tint(SonderPalette.ironGrey)
                }
            }
            .sonderPanel()
        }
    }

    private var playActionTitle: String {
        item.kind == .audiobook ? "Listen" : "Play"
    }

    private var playActionDetail: String {
        item.kind == .audiobook
            ? "Opens the audio player and syncs your listening position back to Sonder."
            : "Opens full-screen playback and syncs watch progress back to Sonder."
    }

    private var progressSection: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text(item.kind == .audiobook ? "Listening Progress" : "Watch Progress")
                .font(.headline)
                .foregroundStyle(SonderPalette.text)
            ProgressView(value: duration > 0 ? seconds / duration : 0)
                .tint(SonderPalette.ironGrey)
            HStack {
                Text(SonderTime.format(seconds))
                    .font(.caption.monospacedDigit())
                    .foregroundStyle(SonderPalette.textLight)
                Slider(value: $seconds, in: 0...max(duration, 1))
                Text(SonderTime.format(duration))
                    .font(.caption.monospacedDigit())
                    .foregroundStyle(SonderPalette.textLight)
            }
            Button {
                Task { await model.updatePlaybackState(for: item, seconds: seconds, duration: duration) }
            } label: {
                Label(model.isSavingProgress ? "Saving" : "Save Progress", systemImage: "checkmark")
            }
            .disabled(model.isSavingProgress)
            .tint(SonderPalette.ashGrey)

            if model.pendingProgressCount > 0 {
                Label(
                    "\(model.pendingProgressCount) offline progress update\(model.pendingProgressCount == 1 ? "" : "s") waiting to sync",
                    systemImage: "arrow.triangle.2.circlepath"
                )
                .font(.caption)
                .foregroundStyle(SonderPalette.textLight)
            }
        }
        .sonderPanel()
    }

    private var metadataSection: some View {
        VStack(alignment: .leading, spacing: 10) {
            MetadataRow(label: "Studio", value: item.studio)
            MetadataRow(label: "Year", value: "\(item.year)")
            MetadataRow(label: "Runtime", value: item.durationSeconds > 0 ? SonderTime.format(item.durationSeconds) : "Not probed")
            MetadataRow(label: "Format", value: item.format.rawValue.uppercased())
            MetadataRow(label: "Resolution", value: item.resolutionLabel)
            MetadataRow(label: "Codec", value: item.probedCodec ?? "Not probed")
            if item.kind == .ebook || item.kind == .audiobook {
                MetadataRow(label: "File check", value: item.bookValidation ?? "Awaiting verification")
                MetadataRow(label: "Cover", value: item.coverSource?.capitalized ?? "Needs artwork")
            }
            if let bitrate = item.probedBitrate {
                MetadataRow(label: "Bitrate", value: "\(bitrate / 1_000_000) Mbps")
            }
            if item.kind == .tvShow {
                MetadataRow(label: "Show", value: item.showTitle ?? item.title)
                MetadataRow(label: "Episode", value: item.episodeCode)
            }
        }
    }

    private var iconName: String {
        switch item.kind {
        case .movie:
            return "film"
        case .tvShow:
            return "tv"
        case .documentary:
            return "camera.metering.matrix"
        case .audiobook:
            return "headphones"
        case .ebook:
            return "book"
        case .all:
            return "rectangle.stack"
        }
    }
}

struct SonderPlaybackScreen: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var model: SonderClientModel
    let item: SonderMediaItem
    let initialSeconds: Double
    let initialDuration: Double
    var onClose: (Double, Double, String?) -> Void

    @State private var player: AVPlayer?
    @State private var playerItem: AVPlayerItem?
    @State private var seconds: Double
    @State private var duration: Double
    @State private var audioOptions: [SonderPlaybackTrackOption] = []
    @State private var subtitleOptions: [SonderPlaybackTrackOption] = [.offSubtitle]
    @State private var selectedAudioTrackID: String?
    @State private var selectedSubtitleTrackID: String?
    @State private var subtitlesEnabled = false
    @State private var isLoading = true
    @State private var loadingMessage = "Starting playback"
    @State private var playbackMessage: String?
    @State private var playbackObservers: [NSObjectProtocol] = []
    @State private var timeObserver: Any?
    @State private var syncTask: Task<Void, Never>?
    @State private var isPlaying = false
    @State private var chapters: [SonderAudiobookChapter] = []
    @State private var progressScope: AudiobookProgressScope = .chapter
    @State private var isShowingChapters = false

    init(
        model: SonderClientModel,
        item: SonderMediaItem,
        initialSeconds: Double,
        initialDuration: Double,
        onClose: @escaping (Double, Double, String?) -> Void
    ) {
        self.model = model
        self.item = item
        self.initialSeconds = initialSeconds
        self.initialDuration = initialDuration
        self.onClose = onClose
        _seconds = State(initialValue: initialSeconds)
        _duration = State(initialValue: initialDuration > 0 ? initialDuration : item.durationSeconds)
    }

    var body: some View {
        ZStack {
            Color.black.ignoresSafeArea()

            if let player {
                NativePlayerController(player: player)
                    .ignoresSafeArea()
            } else if isLoading {
                VStack(spacing: 12) {
                    ProgressView().tint(.white)
                    Text(loadingMessage).multilineTextAlignment(.center).foregroundStyle(.white)
                }
                .padding(24)
            } else {
                ContentUnavailableView("Playback unavailable", systemImage: "play.slash", description: Text(playbackMessage ?? "Sonder could not start this stream."))
                    .foregroundStyle(.white)
            }

            VStack {
                HStack {
                    Button {
                        closePlayer()
                    } label: {
                        Label("Done", systemImage: "xmark")
                            .labelStyle(.iconOnly)
                            .font(.headline)
                            .foregroundStyle(.white)
                            .frame(width: 44, height: 44)
                            .background(.black.opacity(0.45), in: Circle())
                    }

                    Spacer()

                    Text(item.title)
                        .font(.headline)
                        .foregroundStyle(.white)
                        .lineLimit(1)

                    Spacer()

                    Color.clear.frame(width: 44, height: 44)
                }
                .padding(.horizontal, 16)
                .padding(.top, 10)

                Spacer()

                PlaybackControlsBar(
                    seconds: displayedSeconds,
                    duration: displayedDuration,
                    audioOptions: audioOptions,
                    subtitleOptions: subtitleOptions,
                    selectedAudioTrackID: Binding(
                        get: { selectedAudioTrackID },
                        set: { selectAudioTrack(id: $0) }
                    ),
                    selectedSubtitleTrackID: Binding(
                        get: { subtitlesEnabled ? selectedSubtitleTrackID : SonderPlaybackTrackOption.offID },
                        set: { selectSubtitleTrack(id: $0) }
                    ),
                    subtitlesEnabled: Binding(
                        get: { subtitlesEnabled },
                        set: { setSubtitlesEnabled($0) }
                    ),
                    isPlaying: isPlaying,
                    onPlayPause: togglePlayback,
                    onSkipBack: { seek(by: -15) },
                    onSkipForward: { seek(by: 15) },
                    chapterContext: currentChapter.map {
                        AudiobookChapterContext(
                            title: $0.title,
                            position: $0.index + 1,
                            total: chapters.count,
                            bookSeconds: seconds,
                            bookDuration: duration
                        )
                    },
                    progressScope: $progressScope,
                    onShowChapters: { isShowingChapters = true }
                )
                .padding(.horizontal, 12)
                .padding(.bottom, 14)
            }
        }
        .task {
            await startPlayback()
        }
        .onDisappear {
            cleanup()
        }
        .sheet(isPresented: $isShowingChapters) {
            AudiobookChaptersSheet(
                chapters: chapters,
                bookDuration: duration,
                currentSeconds: seconds,
                onSelect: { chapter in
                    seek(to: chapter.startSeconds)
                    isShowingChapters = false
                }
            )
        }
    }

    private func startPlayback() async {
        guard player == nil else { return }
        model.audiobookPlayer.pause()
        isLoading = true
        loadingMessage = "Starting playback"
        playbackMessage = nil

        if item.kind == .audiobook {
            chapters = (try? await model.audiobookChapters(for: item)) ?? []
        }

        do {
            try AVAudioSession.sharedInstance().setCategory(.playback, mode: .moviePlayback, policy: .longFormAudio)
            try AVAudioSession.sharedInstance().setActive(true)
        } catch {
            model.logEvent("Audio session setup failed", detail: error.localizedDescription)
        }

        do {
            let response: SonderPlaybackResponse
            if let offline = model.localPlaybackResponse(for: item, duration: duration) {
                response = offline
            } else {
                response = try await model.startPlayback(for: item, seconds: seconds, duration: duration)
            }
            seconds = response.seconds
            duration = response.duration > 0 ? response.duration : duration
            selectedAudioTrackID = response.audioTrackID
            selectedSubtitleTrackID = response.subtitleTrackID
            subtitlesEnabled = response.subtitlesEnabled ?? (response.subtitleTrackID != nil)

            let fallback = model.resolvedServerURL(from: response.streamURL) ?? model.streamURL(for: item)
            let url: URL
            if let local = model.localMediaURL(for: item) {
                url = local
            } else {
                url = try await model.preferredPlaybackURL(for: item, fallback: fallback) {
                    loadingMessage = "Preparing this movie for your device. You can close this screen; the server will finish preparing it."
                }
            }
            try Task.checkCancellation()
            model.logEvent("Playback start", detail: "\(item.title) - \(url.path)")
            let asset = url.isFileURL ? AVURLAsset(url: url) : model.streamAsset(for: url)
            let newItem = AVPlayerItem(asset: asset)
            playerItem = newItem
            installPlaybackObservers(for: newItem, url: url)

            let newPlayer = AVPlayer(playerItem: newItem)
            newPlayer.appliesMediaSelectionCriteriaAutomatically = false
            addPeriodicSync(to: newPlayer)
            player = newPlayer

            if seconds > 5 {
                newPlayer.seek(
                    to: CMTime(seconds: seconds, preferredTimescale: 600),
                    toleranceBefore: .zero,
                    toleranceAfter: .zero
                ) { _ in }
            }
            newPlayer.play()
            isPlaying = true

            await configureTrackOptions(for: newItem, response: response)
            applyRestoredSelections(response: response)
            isLoading = false
        } catch {
            isLoading = false
            playbackMessage = error.localizedDescription
            model.logEvent("Playback start failed", detail: "\(item.title): \(error.localizedDescription)")
        }
    }

    private func configureTrackOptions(for playerItem: AVPlayerItem, response: SonderPlaybackResponse) async {
        try? await Task.sleep(nanoseconds: 500_000_000)
        let asset = playerItem.asset
        var localAudioOptionsByID: [String: (AVMediaSelectionOption, AVMediaSelectionGroup)] = [:]
        var localSubtitleOptionsByID: [String: (AVMediaSelectionOption, AVMediaSelectionGroup)] = [:]
        let audioGroup = try? await asset.loadMediaSelectionGroup(for: .audible)
        let subtitleGroup = try? await asset.loadMediaSelectionGroup(for: .legible)

        if let group = audioGroup {
            for (index, option) in group.options.enumerated() {
                localAudioOptionsByID["embedded-audio:\(index)"] = (option, group)
            }
            audioOptions = response.audioTracks.map { track in
                let local = localAudioOptionsByID[track.id]
                return SonderPlaybackTrackOption(track: track, fallbackKind: .embeddedAudio, option: local?.0, group: local?.1)
            }
            if audioOptions.isEmpty {
                audioOptions = group.options.enumerated().map { index, option in
                    SonderPlaybackTrackOption(id: "embedded-audio:\(index)", label: option.displayName, languageCode: option.extendedLanguageTag, kind: .embeddedAudio, option: option, group: group)
                }
            }
        }

        var subtitleTracks: [SonderPlaybackTrackOption] = [.offSubtitle]
        if let group = subtitleGroup {
            for (index, option) in group.options.enumerated() {
                localSubtitleOptionsByID["embedded-subtitle:\(index)"] = (option, group)
            }
        }
        let serverSubtitleOptions = response.subtitleTracks.map { track in
            let local = localSubtitleOptionsByID[track.id]
            return SonderPlaybackTrackOption(track: track, fallbackKind: track.kind == .sidecar ? .sidecarSubtitle : .embeddedSubtitle, option: local?.0, group: local?.1)
        }
        subtitleTracks.append(contentsOf: serverSubtitleOptions)
        if subtitleTracks.count == 1, let group = subtitleGroup {
            subtitleTracks.append(contentsOf: group.options.enumerated().map { index, option in
                SonderPlaybackTrackOption(id: "embedded-subtitle:\(index)", label: option.displayName, languageCode: option.extendedLanguageTag, kind: .embeddedSubtitle, option: option, group: group)
            })
        }
        subtitleOptions = subtitleTracks
    }

    private func selectAudioTrack(id: String?) {
        selectedAudioTrackID = id
        guard let track = audioOptions.first(where: { $0.id == id }),
              let option = track.option,
              let group = track.group else {
            syncPlaybackState()
            return
        }
        playerItem?.select(option, in: group)
        syncPlaybackState()
    }

    private func selectSubtitleTrack(id: String?) {
        if id == SonderPlaybackTrackOption.offID || id == nil {
            subtitlesEnabled = false
            if let group = subtitleOptions.compactMap(\.group).first {
                playerItem?.select(nil, in: group)
            }
            syncPlaybackState()
            return
        }

        selectedSubtitleTrackID = id
        subtitlesEnabled = true
        guard let track = subtitleOptions.first(where: { $0.id == id }),
              let option = track.option,
              let group = track.group else {
            syncPlaybackState()
            return
        }
        playerItem?.select(option, in: group)
        syncPlaybackState()
    }

    private func setSubtitlesEnabled(_ isEnabled: Bool) {
        subtitlesEnabled = isEnabled
        if isEnabled == false, let group = subtitleOptions.compactMap(\.group).first {
            playerItem?.select(nil, in: group)
        } else if isEnabled, selectedSubtitleTrackID == nil {
            selectedSubtitleTrackID = subtitleOptions.first(where: { $0.kind != .off })?.id
            selectSubtitleTrack(id: selectedSubtitleTrackID)
            return
        }
        syncPlaybackState()
    }

    private func applyRestoredSelections(response: SonderPlaybackResponse) {
        let audioID = selectedAudioTrackID ?? preferredJapaneseAudioTrackID()
        if let audioID {
            selectedAudioTrackID = audioID
            selectAudioTrack(id: audioID)
        }

        if response.subtitleTrackID == nil, let englishSubtitleID = preferredEnglishSubtitleTrackID() {
            selectedSubtitleTrackID = englishSubtitleID
            subtitlesEnabled = true
        }

        if subtitlesEnabled, let selectedSubtitleTrackID {
            selectSubtitleTrack(id: selectedSubtitleTrackID)
        } else {
            selectSubtitleTrack(id: SonderPlaybackTrackOption.offID)
        }
    }

    private func preferredJapaneseAudioTrackID() -> String? {
        audioOptions.first(where: { $0.matches(languageCode: "ja", labels: ["japanese", "jpn"]) })?.id
            ?? audioOptions.first?.id
    }

    private func preferredEnglishSubtitleTrackID() -> String? {
        subtitleOptions.first(where: { $0.kind != .off && $0.matches(languageCode: "en", labels: ["english", "eng"]) })?.id
    }

    private func addPeriodicSync(to player: AVPlayer) {
        timeObserver = player.addPeriodicTimeObserver(forInterval: CMTime(seconds: 5, preferredTimescale: 600), queue: .main) { time in
            seconds = time.seconds.isFinite ? time.seconds : seconds
            let currentDuration = player.currentItem?.duration.seconds ?? duration
            if currentDuration.isFinite, currentDuration > 0 {
                duration = currentDuration
            }
            syncPlaybackState(debounced: true)
        }
    }

    private func togglePlayback() {
        guard let player else { return }
        if player.rate == 0 {
            player.play()
            isPlaying = true
        } else {
            player.pause()
            isPlaying = false
            syncPlaybackState()
        }
    }

    private func seek(by offset: Double) {
        guard let player else { return }
        let current = player.currentTime().seconds.isFinite ? player.currentTime().seconds : seconds
        let target = min(max(current + offset, 0), max(duration, 0))
        player.seek(to: CMTime(seconds: target, preferredTimescale: 600), toleranceBefore: .zero, toleranceAfter: .zero)
        seconds = target
        syncPlaybackState()
    }

    private func seek(to target: Double) {
        guard let player else { return }
        let clamped = min(max(target, 0), max(duration, 0))
        player.seek(to: CMTime(seconds: clamped, preferredTimescale: 600), toleranceBefore: .zero, toleranceAfter: .zero)
        seconds = clamped
        syncPlaybackState()
    }

    private var currentChapter: SonderAudiobookChapter? {
        chapters.last { seconds >= $0.startSeconds } ?? chapters.first
    }

    private var displayedSeconds: Double {
        guard progressScope == .chapter, let chapter = currentChapter else { return seconds }
        return max(seconds - chapter.startSeconds, 0)
    }

    private var displayedDuration: Double {
        guard progressScope == .chapter, let chapter = currentChapter else { return duration }
        return max(chapter.end(in: duration) - chapter.startSeconds, 0)
    }

    private func syncPlaybackState(debounced: Bool = false) {
        syncTask?.cancel()
        syncTask = Task { @MainActor in
            if debounced {
                try? await Task.sleep(nanoseconds: 350_000_000)
                guard Task.isCancelled == false else { return }
            }
            await model.updatePlaybackState(
                for: item,
                seconds: seconds,
                duration: duration,
                audioTrackID: selectedAudioTrackID,
                subtitleTrackID: selectedSubtitleTrackID,
                subtitlesEnabled: subtitlesEnabled
            )
        }
    }

    private func installPlaybackObservers(for playerItem: AVPlayerItem, url: URL) {
        removePlaybackObservers()
        let center = NotificationCenter.default
        playbackObservers = [
            center.addObserver(forName: AVPlayerItem.failedToPlayToEndTimeNotification, object: playerItem, queue: .main) { notification in
                let error = notification.userInfo?[AVPlayerItemFailedToPlayToEndTimeErrorKey] as? Error
                let detail = error?.localizedDescription ?? "Unknown playback failure"
                playbackMessage = detail
                Task { @MainActor in
                    model.logEvent("Playback failed", detail: "\(item.title): \(detail)")
                }
            },
            center.addObserver(forName: AVPlayerItem.playbackStalledNotification, object: playerItem, queue: .main) { _ in
                playbackMessage = "Playback stalled while loading the stream."
                Task { @MainActor in
                    model.logEvent("Playback stalled", detail: "\(item.title) - \(url.absoluteString)")
                }
            },
            center.addObserver(forName: AVPlayerItem.newErrorLogEntryNotification, object: playerItem, queue: .main) { _ in
                playerItem.fetchErrorLog { log in
                    let event = log?.events.last
                    let detail = event?.errorComment ?? event?.serverAddress ?? "AVPlayer reported a stream error."
                    Task { @MainActor in
                        playbackMessage = detail
                        model.logEvent("Playback error log", detail: "\(item.title): \(detail)")
                    }
                }
            },
            center.addObserver(forName: AVPlayerItem.newAccessLogEntryNotification, object: playerItem, queue: .main) { _ in
                playerItem.fetchAccessLog { log in
                    guard let event = log?.events.last else { return }
                    let detail = "\(Int(event.indicatedBitrate)) bps from \(event.serverAddress ?? url.host ?? "stream")"
                    Task { @MainActor in
                        model.logEvent("Playback access", detail: "\(item.title): \(detail)")
                    }
                }
            }
        ]

        Task { @MainActor in
            try? await Task.sleep(nanoseconds: 1_500_000_000)
            switch playerItem.status {
            case .readyToPlay:
                let durationSeconds = playerItem.duration.seconds.isFinite ? playerItem.duration.seconds : item.durationSeconds
                duration = durationSeconds
                model.logEvent("Playback ready", detail: "\(item.title): \(SonderTime.format(durationSeconds))")
            case .failed:
                let detail = playerItem.error?.localizedDescription ?? "AVPlayer could not load this stream."
                playbackMessage = detail
                model.logEvent("Playback item failed", detail: "\(item.title): \(detail)")
            case .unknown:
                model.logEvent("Playback pending", detail: "\(item.title): player is still resolving the stream")
            @unknown default:
                break
            }
        }
    }

    private func closePlayer() {
        let finalSeconds = player?.currentTime().seconds ?? seconds
        let finalDuration = player?.currentItem?.duration.seconds ?? duration
        if finalSeconds.isFinite {
            seconds = finalSeconds
        }
        if finalDuration.isFinite, finalDuration > 0 {
            duration = finalDuration
        }
        syncPlaybackState()
        onClose(seconds, duration, playbackMessage)
        dismiss()
    }

    private func cleanup() {
        syncTask?.cancel()
        player?.pause()
        isPlaying = false
        if let timeObserver, let player {
            player.removeTimeObserver(timeObserver)
        }
        player = nil
        playerItem = nil
        try? AVAudioSession.sharedInstance().setActive(false, options: .notifyOthersOnDeactivation)
        removePlaybackObservers()
    }

    private func removePlaybackObservers() {
        let center = NotificationCenter.default
        playbackObservers.forEach { center.removeObserver($0) }
        playbackObservers = []
    }
}

struct NativePlayerController: UIViewControllerRepresentable {
    let player: AVPlayer

    func makeUIViewController(context: Context) -> AVPlayerViewController {
        let controller = AVPlayerViewController()
        controller.player = player
        controller.showsPlaybackControls = true
        controller.entersFullScreenWhenPlaybackBegins = true
        controller.exitsFullScreenWhenPlaybackEnds = false
        return controller
    }

    func updateUIViewController(_ uiViewController: AVPlayerViewController, context: Context) {
        uiViewController.player = player
    }
}

struct PlaybackControlsBar: View {
    let seconds: Double
    let duration: Double
    let audioOptions: [SonderPlaybackTrackOption]
    let subtitleOptions: [SonderPlaybackTrackOption]
    @Binding var selectedAudioTrackID: String?
    @Binding var selectedSubtitleTrackID: String?
    @Binding var subtitlesEnabled: Bool
    let isPlaying: Bool
    let onPlayPause: () -> Void
    let onSkipBack: () -> Void
    let onSkipForward: () -> Void
    let chapterContext: AudiobookChapterContext?
    @Binding var progressScope: AudiobookProgressScope
    let onShowChapters: () -> Void

    var body: some View {
        VStack(spacing: 10) {
            ProgressView(value: duration > 0 ? seconds / duration : 0)
                .tint(.white)

            if let chapterContext {
                HStack(spacing: 10) {
                    Picker("Progress scope", selection: $progressScope) {
                        Text("Chapter").tag(AudiobookProgressScope.chapter)
                        Text("Book").tag(AudiobookProgressScope.book)
                    }
                    .pickerStyle(.segmented)

                    Button(action: onShowChapters) {
                        VStack(alignment: .trailing, spacing: 1) {
                            Text("Chapter \(chapterContext.position) of \(chapterContext.total)")
                                .font(.caption.weight(.semibold))
                            Text(chapterContext.title)
                                .font(.caption2)
                                .lineLimit(1)
                        }
                    }
                    .foregroundStyle(.white)
                    .accessibilityLabel("Chapter \(chapterContext.position) of \(chapterContext.total), \(chapterContext.title). Show chapters")
                }

                Text("Book: \(SonderTime.format(chapterContext.bookSeconds)) of \(SonderTime.format(chapterContext.bookDuration))")
                    .font(.caption.monospacedDigit())
                    .foregroundStyle(.white.opacity(0.76))
                    .frame(maxWidth: .infinity, alignment: .leading)
            }

            HStack(spacing: 12) {
                Text("\(SonderTime.format(seconds)) / \(SonderTime.format(duration))")
                    .font(.caption.monospacedDigit())
                    .foregroundStyle(.white.opacity(0.86))

                Spacer()

                if audioOptions.isEmpty == false {
                    Menu {
                        ForEach(audioOptions) { option in
                            Button {
                                selectedAudioTrackID = option.id
                            } label: {
                                Label(option.label, systemImage: selectedAudioTrackID == option.id ? "checkmark" : "speaker.wave.2")
                            }
                        }
                    } label: {
                        Image(systemName: "speaker.wave.2")
                            .frame(width: 44, height: 44)
                    }
                }

                Menu {
                    ForEach(subtitleOptions) { option in
                        Button {
                            selectedSubtitleTrackID = option.id
                        } label: {
                            Label(option.label, systemImage: selectedSubtitleTrackID == option.id ? "checkmark" : "captions.bubble")
                        }
                    }
                    Toggle("Subtitles", isOn: $subtitlesEnabled)
                } label: {
                    Image(systemName: subtitlesEnabled ? "captions.bubble.fill" : "captions.bubble")
                        .frame(width: 44, height: 44)
                }
            }
            .foregroundStyle(.white)

            HStack(spacing: 26) {
                Button(action: onSkipBack) {
                    Image(systemName: "gobackward.15")
                        .font(.title2)
                        .frame(width: 44, height: 44)
                }
                .accessibilityLabel("Back 15 seconds")

                Button(action: onPlayPause) {
                    Image(systemName: isPlaying ? "pause.fill" : "play.fill")
                        .font(.title2)
                        .frame(width: 52, height: 44)
                        .background(.white.opacity(0.18), in: Capsule())
                }
                .accessibilityLabel(isPlaying ? "Pause" : "Play")

                Button(action: onSkipForward) {
                    Image(systemName: "goforward.15")
                        .font(.title2)
                        .frame(width: 44, height: 44)
                }
                .accessibilityLabel("Forward 15 seconds")
            }
            .foregroundStyle(.white)
        }
        .padding(12)
        .background(.black.opacity(0.62), in: RoundedRectangle(cornerRadius: 8))
    }
}

enum AudiobookProgressScope: Hashable {
    case chapter
    case book
}

struct AudiobookChapterContext {
    let title: String
    let position: Int
    let total: Int
    let bookSeconds: Double
    let bookDuration: Double
}

private struct AudiobookChaptersSheet: View {
    @Environment(\.dismiss) private var dismiss
    let chapters: [SonderAudiobookChapter]
    let bookDuration: Double
    let currentSeconds: Double
    let onSelect: (SonderAudiobookChapter) -> Void

    var body: some View {
        NavigationStack {
            List(chapters) { chapter in
                let end = chapter.end(in: bookDuration)
                let isCurrent = currentSeconds >= chapter.startSeconds && currentSeconds < end
                Button { onSelect(chapter) } label: {
                    VStack(alignment: .leading, spacing: 4) {
                        HStack {
                            Text("Chapter \(chapter.index + 1) of \(chapters.count)")
                                .font(.caption.weight(.semibold))
                            Spacer()
                            Text(SonderTime.format(end - chapter.startSeconds))
                                .font(.caption.monospacedDigit())
                        }
                        Text(chapter.title)
                            .font(.body.weight(.semibold))
                            .lineLimit(2)
                        Text("Book position: \(SonderTime.format(chapter.startSeconds)) – \(SonderTime.format(end))")
                            .font(.caption.monospacedDigit())
                    }
                    .foregroundStyle(isCurrent ? SonderPalette.ironGrey : SonderPalette.text)
                }
            }
            .navigationTitle("Chapters")
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button("Done") { dismiss() }
                }
            }
        }
    }
}

struct SonderPlaybackTrackOption: Identifiable, Hashable {
    static let offID = "off"
    static let offSubtitle = SonderPlaybackTrackOption(id: offID, label: "Off", languageCode: nil, kind: .off, option: nil, group: nil)

    var id: String
    var label: String
    var languageCode: String?
    var kind: Kind
    var option: AVMediaSelectionOption?
    var group: AVMediaSelectionGroup?

    enum Kind: Hashable {
        case off
        case embeddedAudio
        case embeddedSubtitle
        case sidecarSubtitle
    }

    init(
        id: String,
        label: String,
        languageCode: String? = nil,
        kind: Kind,
        option: AVMediaSelectionOption?,
        group: AVMediaSelectionGroup?
    ) {
        self.id = id
        self.label = label
        self.languageCode = languageCode
        self.kind = kind
        self.option = option
        self.group = group
    }

    init(track: SonderPlaybackTrack, fallbackKind: Kind, option: AVMediaSelectionOption?, group: AVMediaSelectionGroup?) {
        let displayKind = track.kind == .sidecar ? "Sidecar" : "Embedded"
        let language = track.languageCode.map { " \($0.uppercased())" } ?? ""
        self.id = track.id
        self.label = "\(track.label)\(language) - \(displayKind)"
        self.languageCode = track.languageCode
        self.kind = track.kind == .sidecar ? .sidecarSubtitle : fallbackKind
        self.option = option
        self.group = group
    }

    func matches(languageCode target: String, labels: [String]) -> Bool {
        if languageCode?.localizedCaseInsensitiveCompare(target) == .orderedSame {
            return true
        }
        return labels.contains { label.localizedCaseInsensitiveContains($0) }
    }

    static func == (lhs: SonderPlaybackTrackOption, rhs: SonderPlaybackTrackOption) -> Bool {
        lhs.id == rhs.id
    }

    func hash(into hasher: inout Hasher) {
        hasher.combine(id)
    }
}
