import ACKit
import ACShared
import Foundation

/// Add part (techJob.ts partErrors): a catalog part or a name, 1–999 pieces, its source, and the checklist item it
/// replaces (the old part is disposed of on site unless it is returned).
public struct PartForm: Sendable, Equatable {
    public var code: String?
    public var name = ""
    public var quantity = 1
    public var source = PartSource.vanStock
    public var replaces: ComponentKey?
    public var isChecked = false

    public init() {
        // an empty form
    }

    /// "Filter F-200 · F200 · 4 in van": a catalog part as the picker lists it.
    static func label(_ part: PartCatalogItem) -> String {
        let stock = part.vanStockQuantity.map { " · " + tr("{n} in van", ["n": String(Int($0))]) } ?? ""
        return "\(part.name) · \(part.code)\(stock)"
    }

    func picked(_ catalog: [PartCatalogItem]) -> PartCatalogItem? {
        code.flatMap { code in catalog.first { $0.code == code } }
    }

    /// The form's refusal, once it was tried.
    public func error(catalog: [PartCatalogItem]) -> String? {
        guard isChecked else { return nil }
        let label = (picked(catalog)?.name ?? name).trimmingCharacters(in: .whitespaces)
        if label.isEmpty { return tr("Choose a catalog part or name it") }
        return (1...999).contains(quantity) ? nil : tr("Quantity must be 1–999")
    }

    /// The part, or nil while the form has an error.
    public func part(catalog: [PartCatalogItem]) -> Part? {
        var tried = self
        tried.isChecked = true
        guard tried.error(catalog: catalog) == nil else { return nil }
        let picked = picked(catalog)
        return Part(
            name: (picked?.name ?? name).trimmingCharacters(in: .whitespaces),
            quantity: Double(quantity),
            catalogCode: picked?.code,
            source: source,
            lotSerial: nil,
            replacesComponentKey: replaces,
            oldPartDisposal: replaces == nil ? nil : .disposedOnSite,
            receiptAttachmentId: nil
        )
    }
}

/// A refrigerant record (techJob.ts refrigerantErrors): the cylinder, and the kilograms recovered and charged, 0 or
/// more (an empty field is refused, not read as 0 kg).
public struct RefrigerantForm: Sendable, Equatable {
    public var kind = RefrigerantRecordRefrigerant.r32
    public var cylinder = ""
    public var recovered = ""
    public var charged = ""
    public var leakCheck = RefrigerantRecordLeakCheck.notDone
    public var isChecked = false

    public init() {
        // an empty form
    }

    static func kilograms(_ text: String) -> Double? {
        guard let value = Double(text.trimmingCharacters(in: .whitespaces)), value.isFinite, value >= 0 else { return nil }
        return value
    }

    public var error: String? {
        if cylinder.trimmingCharacters(in: .whitespaces).isEmpty { return tr("The cylinder ID is required") }
        return Self.kilograms(recovered) == nil || Self.kilograms(charged) == nil ? tr("Kilograms, 0 or more") : nil
    }

    public var record: RefrigerantRecord? {
        guard error == nil, let recovered = Self.kilograms(recovered), let charged = Self.kilograms(charged) else { return nil }
        return RefrigerantRecord(
            refrigerant: kind,
            cylinderId: cylinder.trimmingCharacters(in: .whitespaces),
            recoveredKg: recovered,
            chargedKg: charged,
            leakCheck: leakCheck,
            leakCheckMethod: nil
        )
    }
}
