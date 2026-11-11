import jsQR from "jsqr";
import { CameraOff } from "lucide-react";
import { useEffect, useRef, useState } from "react";

/** Live camera view that reads a QR code and reports its text once (RF-68). */
export function QrScanner({ onRead }: { onRead: (text: string) => void }) {
  const video = useRef<HTMLVideoElement>(null);
  const supported = typeof navigator !== "undefined" && !!navigator.mediaDevices?.getUserMedia;
  const [error, setError] = useState(supported ? "" : "Este navegador não libera a câmera. Cole o link da nota abaixo.");
  const done = useRef(onRead);
  useEffect(() => {
    done.current = onRead;
  }, [onRead]);

  // Opens the back camera and decodes frames until a QR code is found; stops the camera on unmount.
  useEffect(() => {
    let stream: MediaStream | undefined;
    let frame = 0;
    let stopped = false;
    const canvas = document.createElement("canvas");
    const ctx = canvas.getContext("2d", { willReadFrequently: true });

    /** Decodes the current frame and schedules the next one. */
    const tick = () => {
      const v = video.current;
      if (stopped || !v || !ctx) return;
      if (v.readyState === v.HAVE_ENOUGH_DATA && v.videoWidth > 0) {
        const scale = Math.min(1, 720 / v.videoWidth);
        canvas.width = Math.round(v.videoWidth * scale);
        canvas.height = Math.round(v.videoHeight * scale);
        ctx.drawImage(v, 0, 0, canvas.width, canvas.height);
        const img = ctx.getImageData(0, 0, canvas.width, canvas.height);
        const code = jsQR(img.data, img.width, img.height, { inversionAttempts: "dontInvert" });
        if (code?.data) {
          stopped = true;
          done.current(code.data);
          return;
        }
      }
      frame = requestAnimationFrame(tick);
    };

    if (!supported) return;
    navigator.mediaDevices
      .getUserMedia({ video: { facingMode: "environment" }, audio: false })
      .then((s) => {
        if (stopped) {
          s.getTracks().forEach((t) => t.stop());
          return;
        }
        stream = s;
        const v = video.current;
        if (!v) return;
        v.srcObject = s;
        void v.play();
        frame = requestAnimationFrame(tick);
      })
      .catch(() => setError("Não consegui abrir a câmera. Permita o acesso nos ajustes do Safari ou cole o link da nota abaixo."));
    return () => {
      stopped = true;
      cancelAnimationFrame(frame);
      stream?.getTracks().forEach((t) => t.stop());
    };
  }, [supported]);

  if (error)
    return (
      <div className="flex aspect-square w-full flex-col items-center justify-center gap-2 rounded-2xl bg-muted p-6 text-center text-sm text-muted-foreground">
        <CameraOff className="size-6" />
        {error}
      </div>
    );
  return (
    <div className="relative aspect-square w-full overflow-hidden rounded-2xl bg-black">
      <video ref={video} className="size-full object-cover" playsInline muted aria-label="Câmera" />
      <div className="pointer-events-none absolute inset-[18%] rounded-2xl border-2 border-white/80 shadow-[0_0_0_9999px_rgb(0_0_0/0.35)]" aria-hidden />
      <p className="absolute inset-x-0 bottom-3 text-center text-xs text-white/90">Aponte para o QR code da nota</p>
    </div>
  );
}
