import AVKit
import SonderAPI
import SwiftUI

struct SonderVideoPlayerView: View {
    let item: SonderMediaItem
    @ObservedObject var library: SonderLibrary
    let close: () -> Void

    @State private var player: AVPlayer?
    @State private var timeObserver: Any?
    @State private var itemStatusObservation: NSKeyValueObservation?
    @State private var playbackFailureObserver: NSObjectProtocol?
    @State private var mediaSelectionTask: Task<Void, Never>?
    @State private var fullScreenCloseObserver: NSObjectProtocol?
    @State private var fullScreenExitObserver: NSObjectProtocol?
    @State private var fullScreenWindow: NSWindow?
    @State private var isFullScreenPlayerActive = false
    @State private var didStartAccessing = false
    @State private var playableURL: URL?
    @State private var playbackError: String?

    var body: some View {
        VStack(spacing: 0) {
            HStack(spacing: 12) {
                VStack(alignment: .leading, spacing: 2) {
                    Text(item.kind == .tvShow ? [item.episodeCode, item.title].joined(separator: " - ") : item.title)
                        .font(.headline)
                        .lineLimit(1)
                    Text(item.subtitle)
                        .font(.caption)
                        .foregroundStyle(SonderTheme.textLight)
                        .lineLimit(1)
                }
                Spacer()
                Button {
                    toggleFullScreen()
                } label: {
                    Label("Full Screen", systemImage: "arrow.up.left.and.arrow.down.right")
                }
                .buttonStyle(.borderless)
                Button {
                    saveProgressAndClose()
                } label: {
                    Label("Close", systemImage: "xmark")
                }
                .keyboardShortcut(.cancelAction)
            }
            .padding(14)
            .background(SonderTheme.surface)

            ZStack {
                SonderTheme.background
                if isFullScreenPlayerActive {
                    VStack(spacing: 12) {
                        Image(systemName: "arrow.up.left.and.arrow.down.right")
                            .font(.system(size: 36, weight: .semibold))
                        Text("Playing full screen")
                            .font(.headline)
                        Text("Close or exit the full-screen player to return playback here.")
                            .font(.caption)
                            .foregroundStyle(SonderTheme.textLight)
                    }
                    .foregroundStyle(SonderTheme.text)
                    .padding()
                } else if let player, playbackError == nil {
                    SonderAVPlayerContainer(player: player)
                        .onAppear { player.play() }
                } else {
                    VStack(spacing: 12) {
                        Image(systemName: "exclamationmark.triangle")
                            .font(.system(size: 36, weight: .semibold))
                        Text("This file could not be loaded in Sonder.")
                        Text(playbackError ?? item.playableURL?.path ?? "Missing media file")
                            .font(.caption)
                            .foregroundStyle(SonderTheme.textLight)
                            .multilineTextAlignment(.center)
                            .lineLimit(4)
                            .truncationMode(.middle)
                    }
                    .foregroundStyle(SonderTheme.text)
                    .padding()
                }
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(SonderTheme.background)
        .onAppear(perform: preparePlayer)
        .onDisappear(perform: tearDownPlayer)
        .onChange(of: item.id) { _, _ in
            tearDownPlayer()
            preparePlayer()
        }
    }

    private func preparePlayer() {
        guard player == nil else { return }
        guard let url = item.playableURL else {
            reportPlaybackFailure("Missing media file")
            return
        }

        playbackError = nil
        playableURL = url
        didStartAccessing = url.startAccessingSecurityScopedResource()
        let playerItem = AVPlayerItem(url: url)
        let avPlayer = AVPlayer(playerItem: playerItem)
        avPlayer.appliesMediaSelectionCriteriaAutomatically = false

        itemStatusObservation = playerItem.observe(\.status, options: [.initial, .new]) { observedItem, _ in
            DispatchQueue.main.async {
                switch observedItem.status {
                case .readyToPlay:
                    applySavedMediaSelection(to: observedItem)
                    if let progress = library.progressRecord(for: item), progress.seconds > 0 {
                        avPlayer.seek(to: CMTime(seconds: progress.seconds, preferredTimescale: 600))
                    }
                    avPlayer.play()
                case .failed:
                    let message = observedItem.error?.localizedDescription ?? avPlayer.error?.localizedDescription ?? "AVPlayer rejected this media file."
                    reportPlaybackFailure(message)
                case .unknown:
                    break
                @unknown default:
                    reportPlaybackFailure("AVPlayer entered an unknown playback state.")
                }
            }
        }

        playbackFailureObserver = NotificationCenter.default.addObserver(
            forName: .AVPlayerItemFailedToPlayToEndTime,
            object: playerItem,
            queue: .main
        ) { notification in
            let error = notification.userInfo?[AVPlayerItemFailedToPlayToEndTimeErrorKey] as? Error
            reportPlaybackFailure(error?.localizedDescription ?? "Playback stopped before the file ended.")
        }

        timeObserver = avPlayer.addPeriodicTimeObserver(forInterval: CMTime(seconds: 10, preferredTimescale: 600), queue: .main) { _ in
            saveProgress()
        }
        player = avPlayer
    }

    private func saveProgressAndClose() {
        saveProgress()
        close()
    }

    private func applySavedMediaSelection(to playerItem: AVPlayerItem) {
        mediaSelectionTask?.cancel()
        let currentItem = library.item(id: item.id) ?? item
        let subtitleTracks = currentItem.embeddedSubtitleTracks
        guard let session = library.playbackSessionSnapshot(
            itemID: item.id,
            audioTracks: currentItem.embeddedAudioTracks,
            subtitleTracks: subtitleTracks
        ) else { return }

        mediaSelectionTask = Task { @MainActor in
            await selectEmbeddedTrack(id: session.audioTrackID, prefix: "embedded-audio", characteristic: .audible, in: playerItem)
            if session.subtitlesEnabled == true {
                await selectEmbeddedTrack(id: session.subtitleTrackID, prefix: "embedded-subtitle", characteristic: .legible, in: playerItem)
            } else if let group = try? await playerItem.asset.loadMediaSelectionGroup(for: .legible) {
                playerItem.select(nil, in: group)
            }
        }
    }

    private func selectEmbeddedTrack(id: String?, prefix: String, characteristic: AVMediaCharacteristic, in playerItem: AVPlayerItem) async {
        guard let id, id.hasPrefix("\(prefix):"),
              let optionIndex = Int(id.dropFirst(prefix.count + 1)),
              let group = try? await playerItem.asset.loadMediaSelectionGroup(for: characteristic) else { return }
        let options = AVMediaSelectionGroup.playableMediaSelectionOptions(from: group.options)
        guard options.indices.contains(optionIndex) else { return }
        playerItem.select(options[optionIndex], in: group)
    }

    private func saveProgress() {
        guard let player else { return }
        let seconds = player.currentTime().seconds
        let duration = player.currentItem?.duration.seconds ?? item.durationSeconds
        guard seconds.isFinite, duration.isFinite, duration > 0 else { return }
        library.savePlaybackProgress(itemID: item.id, seconds: seconds, duration: duration)
    }

    private func tearDownPlayer() {
        saveProgress()
        closeFullScreenWindow()
        if let timeObserver, let player {
            player.removeTimeObserver(timeObserver)
        }
        if let playbackFailureObserver {
            NotificationCenter.default.removeObserver(playbackFailureObserver)
        }
        mediaSelectionTask?.cancel()
        mediaSelectionTask = nil
        itemStatusObservation?.invalidate()
        itemStatusObservation = nil
        playbackFailureObserver = nil
        timeObserver = nil
        player?.pause()
        player = nil
        if didStartAccessing {
            playableURL?.stopAccessingSecurityScopedResource()
        }
        didStartAccessing = false
        playableURL = nil
    }

    private func reportPlaybackFailure(_ message: String) {
        playbackError = message
        player?.pause()
        library.reportPlaybackIssue(
            title: "Playback failed",
            detail: "\(item.title): \(message)"
        )
    }

    private func toggleFullScreen() {
        guard let player else { return }
        if fullScreenWindow != nil {
            closeFullScreenWindow()
            return
        }

        let playerView = AVPlayerView()
        playerView.player = player
        playerView.controlsStyle = .floating
        playerView.videoGravity = .resizeAspect

        let window = NSWindow(
            contentRect: NSScreen.main?.frame ?? NSRect(x: 0, y: 0, width: 1280, height: 720),
            styleMask: [.titled, .closable, .resizable, .fullSizeContentView],
            backing: .buffered,
            defer: false
        )
        window.title = item.kind == .tvShow ? [item.episodeCode, item.title].joined(separator: " - ") : item.title
        window.contentView = playerView
        window.collectionBehavior = [.fullScreenPrimary]
        window.isReleasedWhenClosed = false
        fullScreenCloseObserver = NotificationCenter.default.addObserver(
            forName: NSWindow.willCloseNotification,
            object: window,
            queue: .main
        ) { _ in
            restoreInlinePlayerAfterFullScreen()
        }
        fullScreenExitObserver = NotificationCenter.default.addObserver(
            forName: NSWindow.didExitFullScreenNotification,
            object: window,
            queue: .main
        ) { _ in
            closeFullScreenWindow()
        }
        fullScreenWindow = window
        isFullScreenPlayerActive = true
        window.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)
        window.toggleFullScreen(nil)
        player.play()
    }

    private func closeFullScreenWindow() {
        guard let window = fullScreenWindow else {
            restoreInlinePlayerAfterFullScreen()
            return
        }
        fullScreenWindow = nil
        window.close()
        restoreInlinePlayerAfterFullScreen()
    }

    private func restoreInlinePlayerAfterFullScreen() {
        if let fullScreenCloseObserver {
            NotificationCenter.default.removeObserver(fullScreenCloseObserver)
        }
        if let fullScreenExitObserver {
            NotificationCenter.default.removeObserver(fullScreenExitObserver)
        }
        fullScreenCloseObserver = nil
        fullScreenExitObserver = nil
        fullScreenWindow = nil
        isFullScreenPlayerActive = false
        player?.play()
    }
}

private struct SonderAVPlayerContainer: NSViewRepresentable {
    let player: AVPlayer

    func makeNSView(context: Context) -> AVPlayerView {
        let view = AVPlayerView()
        view.controlsStyle = .floating
        view.videoGravity = .resizeAspect
        view.player = player
        return view
    }

    func updateNSView(_ nsView: AVPlayerView, context: Context) {
        if nsView.player !== player {
            nsView.player = player
        }
    }
}
