import ACKit
import ACShared
import SwiftUI

#if os(iOS)
import PencilKit
import UIKit

/// A photo for the report as JPEG (the library's may be HEIC): at most 2048 pixels on its long side, so it stays under
/// the Core API's 5 MiB.
enum PhotoEncoding {
    static func jpeg(_ data: Data, name: String) -> NewPhoto? {
        guard let image = UIImage(data: data) else { return nil }
        return jpeg(image, name: name)
    }

    static func jpeg(_ image: UIImage, name: String) -> NewPhoto? {
        let width = image.size.width * image.scale
        let height = image.size.height * image.scale
        let longest = max(width, height)
        let scale = longest > 2048 ? 2048 / longest : 1
        let size = CGSize(width: width * scale, height: height * scale)
        // one pixel per point: the default format draws at the screen's scale (3 on most iPhones), which would make a
        // 2048-point photo 6144 pixels wide and its JPEG larger than the Core API's 5 MiB
        let format = UIGraphicsImageRendererFormat()
        format.scale = 1
        let resized = UIGraphicsImageRenderer(size: size, format: format).image { _ in
            image.draw(in: CGRect(origin: .zero, size: size))
        }
        guard let data = resized.jpegData(compressionQuality: 0.8) else { return nil }
        let base = (name as NSString).deletingPathExtension
        return NewPhoto(name: "\(base.isEmpty ? "photo" : base).jpg", mime: .imageJpeg, data: data)
    }
}

/// The camera (UIImagePickerController): the photo taken, or nil when the technician cancels.
struct CameraPicker: UIViewControllerRepresentable {
    let taken: @MainActor (UIImage?) -> Void

    static var isAvailable: Bool { UIImagePickerController.isSourceTypeAvailable(.camera) }

    func makeUIViewController(context: Context) -> UIImagePickerController {
        let picker = UIImagePickerController()
        picker.sourceType = .camera
        picker.delegate = context.coordinator
        return picker
    }

    func updateUIViewController(_ picker: UIImagePickerController, context: Context) {
        // nothing changes once it is shown
    }

    func makeCoordinator() -> Coordinator { Coordinator(taken: taken) }

    @MainActor
    final class Coordinator: NSObject, UIImagePickerControllerDelegate, UINavigationControllerDelegate {
        let taken: @MainActor (UIImage?) -> Void

        init(taken: @escaping @MainActor (UIImage?) -> Void) {
            self.taken = taken
        }

        func imagePickerController(
            _ picker: UIImagePickerController,
            didFinishPickingMediaWithInfo info: [UIImagePickerController.InfoKey: Any]
        ) {
            taken(info[.originalImage] as? UIImage)  // UIKit hands the picked media as Any (Apple's API)
        }

        func imagePickerControllerDidCancel(_ picker: UIImagePickerController) {
            taken(nil)
        }
    }
}

/// The customer's signature pad (PencilKit): dark ink on a light pad in either appearance.
struct SignaturePad: UIViewRepresentable {
    @Binding var drawing: PKDrawing

    func makeUIView(context: Context) -> PKCanvasView {
        let canvas = PKCanvasView()
        canvas.drawingPolicy = .anyInput
        canvas.tool = PKInkingTool(.pen, color: .black, width: 3)
        canvas.backgroundColor = .white
        canvas.overrideUserInterfaceStyle = .light
        canvas.delegate = context.coordinator
        canvas.accessibilityLabel = tr("Signature pad")
        canvas.accessibilityIdentifier = "signOff.pad"
        return canvas
    }

    func updateUIView(_ canvas: PKCanvasView, context: Context) {
        if canvas.drawing != drawing { canvas.drawing = drawing }
    }

    func makeCoordinator() -> Coordinator { Coordinator(drawing: $drawing) }

    final class Coordinator: NSObject, PKCanvasViewDelegate {
        let drawing: Binding<PKDrawing>

        init(drawing: Binding<PKDrawing>) {
            self.drawing = drawing
        }

        func canvasViewDrawingDidChange(_ canvas: PKCanvasView) {
            drawing.wrappedValue = canvas.drawing
        }
    }

    /// The signature as PNG on white, or nil when nothing is drawn.
    static func png(_ drawing: PKDrawing) -> Data? {
        guard !drawing.strokes.isEmpty else { return nil }
        let bounds = drawing.bounds.insetBy(dx: -12, dy: -12)
        let ink = drawing.image(from: bounds, scale: 2)
        return UIGraphicsImageRenderer(size: bounds.size).image { context in
            UIColor.white.setFill()
            context.fill(CGRect(origin: .zero, size: bounds.size))
            ink.draw(at: .zero)
        }
        .pngData()
    }
}

/// A report photo full size, from its bytes.
struct PhotoImage: View {
    let data: Data
    /// The photo's file name, its label.
    let name: String

    static func isImage(_ data: Data) -> Bool { UIImage(data: data) != nil }

    var body: some View {
        if let image = UIImage(data: data) {
            Image(uiImage: image).resizable().scaledToFit().accessibilityLabel(Text(verbatim: name))
        }
    }
}
#else
import PencilKit

struct PhotoImage: View {
    let data: Data
    /// The photo's file name, its label.
    let name: String

    static func isImage(_ data: Data) -> Bool { !data.isEmpty }

    var body: some View { EmptyView() }
}

/// The package's tests build for macOS: the screens' iOS pieces are stand-ins there.
enum PhotoEncoding {
    static func jpeg(_ data: Data, name: String) -> NewPhoto? { NewPhoto(name: name, mime: .imageJpeg, data: data) }
}

struct CameraPicker: View {
    let taken: @MainActor (Data?) -> Void
    static let isAvailable = false

    var body: some View { EmptyView() }
}

struct SignaturePad: View {
    @Binding var drawing: PKDrawing

    var body: some View { EmptyView() }

    static func png(_ drawing: PKDrawing) -> Data? { drawing.strokes.isEmpty ? nil : drawing.dataRepresentation() }
}
#endif
