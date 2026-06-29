import Foundation

nonisolated struct SonderAudiobookPlaybackSnapshot: Codable, Hashable, Sendable {
    var seconds: Double
    var duration: Double
    var percent: Double
    var currentChapterIndex: Int?
    var currentChapterTitle: String?
    var currentChapterStartSeconds: Double?
    var currentChapterEndSeconds: Double?
    var nextChapterStartSeconds: Double?
}

nonisolated enum SonderAudiobookPlaybackModel {
    static func resolvedChapters(_ chapters: [SonderAudiobookChapterRecord], duration: Double) -> [SonderAudiobookChapterRecord] {
        let sorted = chapters.sorted { lhs, rhs in
            if lhs.startSeconds == rhs.startSeconds { return lhs.index < rhs.index }
            return lhs.startSeconds < rhs.startSeconds
        }
        guard sorted.isEmpty == false else {
            return [SonderAudiobookChapterRecord(index: 1, title: "Chapter 1", startSeconds: 0, endSeconds: max(duration, 1))]
        }

        return sorted.enumerated().map { offset, chapter in
            let nextStart = sorted.dropFirst(offset + 1).first?.startSeconds
            let fallbackEnd = nextStart ?? max(duration, chapter.startSeconds)
            let endSeconds = chapter.endSeconds.map { max($0, chapter.startSeconds) } ?? fallbackEnd
            return SonderAudiobookChapterRecord(
                index: chapter.index,
                title: chapter.title,
                startSeconds: max(chapter.startSeconds, 0),
                endSeconds: endSeconds
            )
        }
    }

    static func playback(progress: SonderProgress?, itemDuration: Double, chapters: [SonderAudiobookChapterRecord]) -> SonderAudiobookPlaybackSnapshot {
        let duration = max(progress?.duration ?? itemDuration, itemDuration, 1)
        let seconds = min(max(progress?.seconds ?? 0, 0), duration)
        let resolved = resolvedChapters(chapters, duration: duration)
        let current = resolved.last { chapter in
            seconds >= chapter.startSeconds && seconds < (chapter.endSeconds ?? duration)
        } ?? resolved.last
        let next = resolved.first { $0.startSeconds > seconds }

        return SonderAudiobookPlaybackSnapshot(
            seconds: seconds,
            duration: duration,
            percent: duration > 0 ? min(max(seconds / duration, 0), 1) : 0,
            currentChapterIndex: current?.index,
            currentChapterTitle: current?.title,
            currentChapterStartSeconds: current?.startSeconds,
            currentChapterEndSeconds: current?.endSeconds,
            nextChapterStartSeconds: next?.startSeconds
        )
    }
}
