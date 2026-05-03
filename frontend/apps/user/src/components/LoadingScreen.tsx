export function LoadingScreen() {
  return (
    <div className="grid h-full place-items-center bg-surface-bg">
      <div className="flex flex-col items-center gap-3">
        <div className="h-10 w-10 rounded-pill border-2 border-klein-500/30 border-t-klein-500 animate-spin" />
        <span className="text-small text-text-tertiary">加载中…</span>
      </div>
    </div>
  );
}
