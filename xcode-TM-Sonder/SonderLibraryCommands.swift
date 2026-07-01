import Foundation

nonisolated struct SonderLibraryActivityDraft: Hashable, Sendable {
    var title: String
    var detail: String
    var icon: String
}

nonisolated struct SonderRenamePlan: Hashable {
    var sourceURL: URL
    var destinationURL: URL
}

nonisolated struct SonderStorageUpdateResult: Hashable, Sendable {
    var storagePath: String?
    var storageBookmark: Data?
    var activity: SonderLibraryActivityDraft
    var didUpdate: Bool
}

nonisolated struct SonderStorageCommandService: Sendable {
    var store: SonderStore

    func storageUpdate(from result: Result<[URL], Error>) -> SonderStorageUpdateResult {
        guard case .success(let urls) = result, let url = urls.first else {
            return SonderStorageUpdateResult(
                storagePath: nil,
                storageBookmark: nil,
                activity: SonderLibraryActivityDraft(
                    title: "Storage unchanged",
                    detail: "The selected folder could not be read.",
                    icon: "exclamationmark.triangle"
                ),
                didUpdate: false
            )
        }

        do {
            return SonderStorageUpdateResult(
                storagePath: url.path,
                storageBookmark: try store.bookmark(for: url),
                activity: SonderLibraryActivityDraft(
                    title: "Updated storage",
                    detail: url.path,
                    icon: "externaldrive"
                ),
                didUpdate: true
            )
        } catch {
            return SonderStorageUpdateResult(
                storagePath: nil,
                storageBookmark: nil,
                activity: SonderLibraryActivityDraft(
                    title: "Storage unchanged",
                    detail: error.localizedDescription,
                    icon: "exclamationmark.triangle"
                ),
                didUpdate: false
            )
        }
    }
}

nonisolated enum SonderPlaybackCommands {
    static func progress(for item: SonderMediaItem, record: SonderProgress?) -> Double {
        record?.percent ?? item.progress
    }

    @MainActor
    static func progressLabel(for item: SonderMediaItem, record: SonderProgress?) -> String {
        let seconds = record?.seconds ?? item.progressSeconds
        let duration = record?.duration ?? item.durationSeconds
        return "\(SonderTime.format(seconds)) of \(SonderTime.format(duration))"
    }

    static func progressUpdate(
        itemID: UUID,
        seconds: Double,
        duration: Double,
        audioTrackID: String? = nil,
        subtitleTrackID: String? = nil,
        subtitlesEnabled: Bool? = nil,
        records: [SonderProgress]
    ) -> (records: [SonderProgress], seconds: Double, duration: Double) {
        SonderProgressRecords.upserting(
            itemID: itemID,
            seconds: seconds,
            duration: duration,
            audioTrackID: audioTrackID,
            subtitleTrackID: subtitleTrackID,
            subtitlesEnabled: subtitlesEnabled,
            in: records
        )
    }

    @MainActor
    static func open(_ item: SonderMediaItem, using systemServices: SonderSystemServicing) -> SonderLibraryActivityDraft? {
        guard let url = item.playableURL else { return nil }
        systemServices.openSecurityScoped(url)
        return SonderLibraryActivityDraft(
            title: "Opened \(item.title)",
            detail: url.lastPathComponent,
            icon: "play.fill"
        )
    }
}

nonisolated enum SonderFileCommandPlanner {
    static func renameForPlexPlan(for item: SonderMediaItem) -> SonderRenamePlan? {
        guard let sourceURL = item.playableURL else { return nil }
        return SonderRenamePlan(
            sourceURL: sourceURL,
            destinationURL: sourceURL.deletingLastPathComponent().appendingPathComponent(item.plexFileName)
        )
    }
}

nonisolated struct SonderRenameCommandResult: Hashable, Sendable {
    var itemID: UUID?
    var sourcePath: String?
    var format: SonderMediaFormat?
    var activity: SonderLibraryActivityDraft
    var didRename: Bool
}

nonisolated struct SonderConversionCompletion: Hashable, Sendable {
    var jobID: UUID
    var itemID: UUID
    var outputPath: String
    var outputFileName: String
    var sourceFileName: String
    var didConvert: Bool
}

nonisolated struct SonderFileCommandService: Sendable {
    var store: SonderStore

    func renameForPlex(_ item: SonderMediaItem) -> SonderRenameCommandResult {
        guard let plan = SonderFileCommandPlanner.renameForPlexPlan(for: item) else {
            return SonderRenameCommandResult(
                itemID: nil,
                sourcePath: nil,
                format: nil,
                activity: SonderLibraryActivityDraft(
                    title: "Rename failed",
                    detail: "\(item.title) has no local file.",
                    icon: "exclamationmark.triangle"
                ),
                didRename: false
            )
        }

        do {
            let finalURL = try store.moveAvoidingCollision(from: plan.sourceURL, to: plan.destinationURL)
            return SonderRenameCommandResult(
                itemID: item.id,
                sourcePath: finalURL.path,
                format: SonderMediaFormat(url: finalURL),
                activity: SonderLibraryActivityDraft(
                    title: "Renamed for Plex",
                    detail: finalURL.lastPathComponent,
                    icon: "textformat"
                ),
                didRename: true
            )
        } catch {
            return SonderRenameCommandResult(
                itemID: item.id,
                sourcePath: nil,
                format: nil,
                activity: SonderLibraryActivityDraft(
                    title: "Rename failed",
                    detail: error.localizedDescription,
                    icon: "exclamationmark.triangle"
                ),
                didRename: false
            )
        }
    }

    func conversionPlan(for item: SonderMediaItem) -> SonderConversionPlan? {
        guard let sourceURL = item.playableURL else { return nil }
        let outputURL = store.availableConversionURL(for: sourceURL)
        return SonderConversionCommands.plan(for: item, outputURL: outputURL)
    }

    func convert(_ plan: SonderConversionPlan) async -> SonderConversionCompletion {
        let didConvert = await store.convertToMP4(sourceURL: plan.sourceURL, outputURL: plan.outputURL)
        return SonderConversionCompletion(
            jobID: plan.job.id,
            itemID: plan.itemID,
            outputPath: plan.outputURL.path,
            outputFileName: plan.outputURL.lastPathComponent,
            sourceFileName: plan.sourceURL.lastPathComponent,
            didConvert: didConvert
        )
    }
}

nonisolated enum SonderConversionCommands {
    static let maxConcurrentConversions = 2

    static func candidates(from items: [SonderMediaItem]) -> [SonderMediaItem] {
        SonderConversionPlanner.candidates(from: items)
    }

    static func plan(for item: SonderMediaItem, outputURL: URL) -> SonderConversionPlan? {
        SonderConversionPlanner.plan(for: item, outputURL: outputURL)
    }
}
