import Foundation

/// Go encodes nil slices as null; both null and an omitted optional collection
/// represent an empty collection on the wire.
@propertyWrapper
public struct DefaultEmptyArray<Element: Codable & Hashable & Sendable>: Codable, Hashable, Sendable {
    public var wrappedValue: [Element]
    public init(wrappedValue: [Element]) { self.wrappedValue = wrappedValue }
    public init(from decoder: Decoder) throws {
        let container = try decoder.singleValueContainer()
        wrappedValue = container.decodeNil() ? [] : try container.decode([Element].self)
    }
    public func encode(to encoder: Encoder) throws {
        var container = encoder.singleValueContainer()
        try container.encode(wrappedValue)
    }
}

extension KeyedDecodingContainer {
    public func decode<T>(_ type: DefaultEmptyArray<T>.Type, forKey key: Key) throws -> DefaultEmptyArray<T> {
        try decodeIfPresent(type, forKey: key) ?? DefaultEmptyArray(wrappedValue: [])
    }
}
