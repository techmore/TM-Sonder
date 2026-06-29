import SwiftUI

struct ProgressRow: View {
    let item: SonderMediaItem
    let progress: SonderProgress

    var body: some View {
        VStack(alignment: .leading, spacing: 5) {
            HStack {
                Text(item.title)
                Spacer()
                Text("\(Int(progress.percent * 100))%")
                    .foregroundStyle(SonderTheme.textLight)
            }
            ProgressView(value: progress.percent)
                .tint(SonderTheme.accentStrong)
        }
    }
}

struct MetricCard: View {
    let title: String
    let value: String
    let detail: String

    var body: some View {
        VStack(alignment: .leading, spacing: 7) {
            Text(title)
                .font(.caption.weight(.semibold))
                .foregroundStyle(SonderTheme.textLight)
            Text(value)
                .font(.system(size: 32, weight: .bold, design: .rounded))
                .foregroundStyle(SonderTheme.text)
            Text(detail)
                .font(.caption)
                .foregroundStyle(SonderTheme.textLight)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(14)
        .background(SonderTheme.surface, in: RoundedRectangle(cornerRadius: 8))
        .overlay(RoundedRectangle(cornerRadius: 8).stroke(SonderTheme.border))
    }
}

struct DashboardPanel<Content: View>: View {
    let title: String
    @ViewBuilder let content: Content

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text(title)
                .font(.headline)
            content
        }
        .frame(maxWidth: .infinity, alignment: .topLeading)
        .padding(14)
        .background(SonderTheme.surface, in: RoundedRectangle(cornerRadius: 8))
        .overlay(RoundedRectangle(cornerRadius: 8).stroke(SonderTheme.border))
    }
}

struct MeterRow: View {
    let label: String
    let value: Int
    let max: Int

    var body: some View {
        VStack(alignment: .leading, spacing: 5) {
            HStack {
                Text(label)
                Spacer()
                Text("\(value)")
                    .foregroundStyle(SonderTheme.textLight)
            }
            GeometryReader { proxy in
                RoundedRectangle(cornerRadius: 3)
                    .fill(SonderTheme.accentMuted)
                    .overlay(alignment: .leading) {
                        RoundedRectangle(cornerRadius: 3)
                            .fill(SonderTheme.accentStrong)
                            .frame(width: proxy.size.width * CGFloat(value) / CGFloat(max))
                    }
            }
            .frame(height: 8)
        }
        .font(.caption)
    }
}

struct EndpointRow: View {
    let label: String
    let value: String

    var body: some View {
        HStack {
            Text(label)
                .foregroundStyle(SonderTheme.textLight)
                .frame(width: 92, alignment: .leading)
            Text(value)
                .font(.body.monospaced())
                .textSelection(.enabled)
        }
    }
}

struct MetadataRow: View {
    let label: String
    let value: String

    var body: some View {
        HStack {
            Text(label)
                .foregroundStyle(SonderTheme.textLight)
                .frame(width: 86, alignment: .leading)
            Text(value)
        }
    }
}

struct PageHeader: View {
    let title: String
    let subtitle: String

    var body: some View {
        VStack(alignment: .leading, spacing: 5) {
            Text(title)
                .font(.largeTitle.weight(.bold))
                .foregroundStyle(SonderTheme.text)
            Text(subtitle)
                .foregroundStyle(SonderTheme.textLight)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

struct StatRow: View {
    let label: String
    let value: String
    let icon: String

    var body: some View {
        HStack {
            Label(label, systemImage: icon)
            Spacer()
            Text(value)
                .foregroundStyle(SonderTheme.textLight)
        }
    }
}

struct AboutPanel: View {
    let title: String
    let icon: String
    let bodyText: String

    var body: some View {
        HStack(alignment: .top, spacing: 14) {
            Image(systemName: icon)
                .font(.title2.weight(.semibold))
                .foregroundStyle(SonderTheme.accentStrong)
                .frame(width: 28)

            VStack(alignment: .leading, spacing: 8) {
                Text(title)
                    .font(.headline)
                    .foregroundStyle(SonderTheme.text)
                Text(bodyText)
                    .font(.body)
                    .foregroundStyle(SonderTheme.textLight)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
        .padding(16)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(SonderTheme.surface, in: RoundedRectangle(cornerRadius: 8))
        .overlay(RoundedRectangle(cornerRadius: 8).stroke(SonderTheme.border))
    }
}

struct TagCloud: View {
    let tags: [String]

    var body: some View {
        FlowLayout {
            ForEach(tags, id: \.self) { tag in
                Text(tag)
                    .font(.caption.weight(.semibold))
                    .padding(.horizontal, 8)
                    .padding(.vertical, 5)
                    .background(SonderTheme.accentMuted, in: Capsule())
                    .foregroundStyle(SonderTheme.accent)
            }
        }
    }
}

struct FlowLayout<Content: View>: View {
    @ViewBuilder let content: Content

    var body: some View {
        WrappingLayout(horizontalSpacing: 8, verticalSpacing: 8) {
            content
        }
    }
}

struct WrappingLayout: Layout {
    var horizontalSpacing: CGFloat = 8
    var verticalSpacing: CGFloat = 8

    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        let maxWidth = proposal.width ?? 600
        let rows = rows(in: maxWidth, subviews: subviews)
        return CGSize(width: maxWidth, height: rows.last.map { $0.y + $0.height } ?? 0)
    }

    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        for item in rows(in: bounds.width, subviews: subviews) {
            subviews[item.index].place(
                at: CGPoint(x: bounds.minX + item.x, y: bounds.minY + item.y),
                proposal: ProposedViewSize(width: item.width, height: item.height)
            )
        }
    }

    private func rows(in maxWidth: CGFloat, subviews: Subviews) -> [(index: Int, x: CGFloat, y: CGFloat, width: CGFloat, height: CGFloat)] {
        var result: [(Int, CGFloat, CGFloat, CGFloat, CGFloat)] = []
        var x: CGFloat = 0
        var y: CGFloat = 0
        var rowHeight: CGFloat = 0

        for index in subviews.indices {
            let size = subviews[index].sizeThatFits(.unspecified)
            if x > 0, x + size.width > maxWidth {
                x = 0
                y += rowHeight + verticalSpacing
                rowHeight = 0
            }
            result.append((index, x, y, min(size.width, maxWidth), size.height))
            x += size.width + horizontalSpacing
            rowHeight = max(rowHeight, size.height)
        }
        return result
    }
}
