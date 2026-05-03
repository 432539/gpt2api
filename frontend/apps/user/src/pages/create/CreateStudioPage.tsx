import { useEffect, useMemo, useRef, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useLocation, useNavigate } from 'react-router-dom';
import {
  ArrowUp,
  Check,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  FileImage,
  Image,
  Loader2,
  Maximize2,
  Mic,
  MoreHorizontal,
  Paperclip,
  Play,
  Sparkles,
  Trash2,
  Video,
  X,
} from 'lucide-react';
import clsx from 'clsx';

import { useEnsureLoggedIn } from '../../hooks/useEnsureLoggedIn';
import { ApiError } from '../../lib/api';
import { fmtRelative } from '../../lib/format';
import { genApi } from '../../lib/services';
import type { GenerationTask, PublicModel } from '../../lib/types';
import { useAuthStore } from '../../stores/auth';
import { toast } from '../../stores/toast';

type StudioMode = 'image' | 'text' | 'video';

const MODES: Array<{ value: StudioMode; label: string; icon: typeof Image }> = [
  { value: 'image', label: '图片', icon: Image },
  { value: 'text', label: '文字', icon: Sparkles },
  { value: 'video', label: '视频', icon: Video },
];

const GENERATING_PHRASES = [
  '正在为您设计中...',
  '灵感正在慢慢成形',
  '细节正在被认真打磨',
  '画面很快就会出现',
];

const IMAGE_MODELS = [
  { code: 'gpt-image-2', label: 'GPT Image 2', cost: 0 },
];

type SelectModel = {
  code: string;
  label: string;
  cost?: number;
  input?: number;
  output?: number;
};

const VIDEO_MODELS = [
  { code: 'grok-imagine-video', label: 'Grok Imagine 视频', cost: 20 },
  { code: 'vid-i2v', label: 'Grok 图生视频', cost: 20 },
];

const TEXT_MODELS = [
  { code: 'grok-4.20-fast', label: 'Grok Fast', input: 1, output: 3 },
  { code: 'grok-4.20-auto', label: 'Grok Auto', input: 1.5, output: 4.5 },
  { code: 'grok-4.20-expert', label: 'Grok Expert', input: 2, output: 6 },
  { code: 'grok-4.20-heavy', label: 'Grok Heavy', input: 4, output: 12 },
  { code: 'gpt-4o-mini', label: 'GPT 4o mini', input: 1, output: 3 },
];

const IMAGE_RATIOS = ['1:1', '3:2', '2:3', '4:3', '3:4', '5:4', '4:5', '16:9', '9:16', '21:9'] as const;
const IMAGE_RESOLUTIONS = ['1K', '2K', '4K'] as const;
const VIDEO_RATIOS = ['16:9', '9:16', '1:1'] as const;
const VIDEO_DURATIONS = [6, 10] as const;
const HISTORY_PAGE_SIZES = [20, 50, 100] as const;
type HistoryDeleteScope = 'before_3d' | 'before_7d' | 'all';
const TEXT_MAX_ATTACHMENTS = 5;
const VIDEO_MAX_ATTACHMENTS = 7;
const SUGGESTIONS = [
  {
    title: '极简产品广告',
    image: '/examples/case-1.jpg',
    prompt: `A minimalist product advertisement with a {argument name="product" default="fried chicken bucket"} placed on a clean white podium.

Background: soft gradient ({argument name="background gradient" default="light cream to white"}), clean studio.

Lighting: soft diffused, premium Apple-style.

Typography (center): "{argument name="headline" default="PURE CRUNCH"}"

Small text below: "Nothing extra. Just perfection."

Style: ultra clean, editorial minimal, high-end branding, 8K.`,
  },
  {
    title: '城市海报',
    image: '/examples/case-2.jpg',
    prompt: `A striking Spring 2026 city poster for Boston with an elegant celebratory mood and a bold contemporary design. On a clean off-white textured background with large areas of negative space, a miniature single sculler rows across the lower right corner of the image on a narrow ribbon of reflective water. The wake from the oar sweeps upward in a dynamic calligraphic curve, gradually transforming into the Charles River and then into a dreamlike hand-painted panorama of Boston. Inside this flowing river-shaped composition are iconic Boston elements: the Back Bay skyline, Beacon Hill brownstones, Acorn Street, Boston Public Garden, Swan Boats, Zakim Bridge, Fenway-inspired details, historic brick architecture, harbor ferries, and the city's waterfront atmosphere. Soft morning fog, golden spring light, subtle festive accents in crimson and gold, rich detail, layered depth, sophisticated city-poster aesthetics, fresh and refined, visually powerful but not overcrowded. Elegant typography in the lower left reads "SPRING 2026" with a vertical slogan "BOSTON, A CITY OF RIVER, MEMORY, AND INVENTION", text clear and beautifully composed, premium graphic design, 9:16`,
  },
  {
    title: '3D 手办工作流',
    image: '/examples/case-3.jpg',
    prompt: `Photorealistic high-quality studio photo of a modern digital art workspace, showing the concept of "from 3D virtual character to real collectible figure."

In the foreground, a highly realistic collectible figurine of [Character Name / Character Identity] is placed on a round wooden display stand. The character has [facial features / appearance], [hairstyle], and a [expression / personality vibe]. The figure is wearing [outfit / costume]. The overall design is refined, premium, and instantly recognizable. The figurine should have realistic collectible statue quality, with subtle resin/sculpture material feel, while still looking highly believable and visually realistic.

The pose is [character pose], natural, stable, elegant, and display-worthy. Shot from a low-angle close-up perspective with slight wide-angle distortion, vertical composition, emphasizing the full figure, clothing structure, leg lines, and pose.

In the background, there is a professional 3D character design workstation with two large curved monitors. Both monitors must show the exact same character as the foreground figurine - same face, same hairstyle, same outfit, same pose, and same overall vibe - clearly expressing the idea of turning a digital 3D character into a real physical figure.

The left monitor shows a gray sculpt / clay model view in a professional 3D sculpting software interface, similar to ZBrush. The gray model must match the foreground figure exactly in character design, pose, outfit structure, and facial identity.

The right monitor shows the fully rendered colored version of the same character, also matching the foreground figure exactly in face, hairstyle, outfit, pose, and temperament. Together, the two monitors reinforce the workflow of "digital character design -> physical collectible statue."

On the desk are a keyboard, mouse, monitor arms, drawing tablet, stylus, and other 3D modeling tools. The workspace is clean, professional, and visually premium. Optional extra elements: [weapon / accessories / theme props / IP-style design details].

Lighting is a mix of soft studio lighting and indoor workspace lighting. The foreground figurine is evenly lit with clear facial and material detail, while the monitors emit cool-toned tech light. Overall mood is realistic, clean, premium, slightly shallow depth of field, ultra-detailed, emphasizing the collectible figure quality, professional 3D design studio atmosphere, and the visual concept of "from digital model to real figure."

photorealistic, ultra detailed, cinematic studio lighting, realistic figurine, collectible statue, 3D character design studio, from digital model to real figure, vertical composition`,
  },
  {
    title: '鹿鼎记海报',
    image: '/examples/case-4.jpg',
    prompt: '生成鹿鼎记海报，展现韦小宝跟老婆XXX，忠于原著的描述，夸大特点，强调女性的美艳和男性的气质',
  },
  {
    title: '人类演化图',
    image: '/examples/case-5.jpg',
    prompt: `{
  "type": "evolutionary timeline infographic",
  "instruction": "Using REFERENCE_0 as a structural base, transform the flat vector design into a highly realistic 3D infographic. Replace the smooth ramps with distinct stone steps and upgrade all organisms to photorealistic 3D models.",
  "style": {
    "background": "{argument name=\\"background style\\" default=\\"vintage textured parchment paper\\"}",
    "staircase": "{argument name=\\"staircase material\\" default=\\"realistic textured stone blocks\\"}",
    "subjects": "{argument name=\\"organism style\\" default=\\"highly detailed photorealistic 3D renders\\"}"
  },
  "layout": {
    "main_title": "{argument name=\\"main title\\" default=\\"人类演化\\"}",
    "sections": [
      {
        "position": "left sidebar",
        "count": 8,
        "labels": ["L0: 单细胞生命", "L1: 多细胞生物", "L2: 动物界", "L3: 脊索动物", "L4: 上陆革命", "L5: 哺乳纲", "L6: 人科演化", "L7: 智人纪元"]
      },
      {
        "position": "top right",
        "title": "获得的功能 / 失去的功能",
        "description": "Legend with plus and minus icons"
      },
      {
        "position": "bottom center",
        "title": "演化关键里程碑",
        "count": 6,
        "description": "Timeline with a silhouette graphic of 6 figures showing ape-to-human evolution"
      }
    ],
    "centerpiece": {
      "description": "Winding stone staircase with 25 numbered steps featuring specific organisms.",
      "count": 25,
      "notable_elements": [
        "Step 07: Jellyfish",
        "Step 09: Ammonite",
        "Step 10: Trilobite",
        "Step 24: Walking human",
        "Step 25: {argument name=\\"future evolution concept\\" default=\\"glowing cosmic silhouette with a question mark\\"}"
      ]
    }
  }
}`,
  },
];

export default function CreateStudioPage() {
  const location = useLocation();
  const navigate = useNavigate();
  const qc = useQueryClient();
  const ensureLoggedIn = useEnsureLoggedIn();
  const refreshMe = useAuthStore((s) => s.refreshMe);
  const token = useAuthStore((s) => s.token);

  const modelCatalog = useQuery({
    queryKey: ['gen.models'],
    queryFn: () => genApi.models(),
    staleTime: 60_000,
  });

  const imageModels = useMemo(() => modelsByKind(modelCatalog.data, 'image', IMAGE_MODELS), [modelCatalog.data]);
  const textModels = useMemo(() => modelsByKind(modelCatalog.data, 'text', TEXT_MODELS), [modelCatalog.data]);
  const videoModels = useMemo(() => modelsByKind(modelCatalog.data, 'video', VIDEO_MODELS), [modelCatalog.data]);

  const mode = modeFromPath(location.pathname);
  const [prompt, setPrompt] = useState('');
  const [textModel, setTextModel] = useState(TEXT_MODELS[0]!.code);
  const [imageModel, setImageModel] = useState(IMAGE_MODELS[0]!.code);
  const [videoModel, setVideoModel] = useState(VIDEO_MODELS[0]!.code);
  const [imageRatio, setImageRatio] = useState<(typeof IMAGE_RATIOS)[number]>('1:1');
  const [imageResolution, setImageResolution] = useState<(typeof IMAGE_RESOLUTIONS)[number]>('1K');
  const [videoRatio, setVideoRatio] = useState<(typeof VIDEO_RATIOS)[number]>('16:9');
  const [count, setCount] = useState(1);
  const [duration, setDuration] = useState<(typeof VIDEO_DURATIONS)[number]>(6);
  const [attachments, setAttachments] = useState<Array<{ id: string; name: string; dataUrl: string }>>([]);
  const [textResult, setTextResult] = useState('');
  const [task, setTask] = useState<GenerationTask | null>(null);
  const [historyPageSize, setHistoryPageSize] = useState<(typeof HISTORY_PAGE_SIZES)[number]>(20);
  const [preview, setPreview] = useState<{ url: string; type: 'image' | 'video'; title: string } | null>(null);
  const pollRef = useRef<number | null>(null);
  const promptRef = useRef<HTMLTextAreaElement | null>(null);
  const fileInputRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => () => {
    if (pollRef.current) window.clearInterval(pollRef.current);
  }, []);

  useEffect(() => {
    setTask(null);
    setTextResult('');
    setAttachments([]);
  }, [mode]);

  useEffect(() => {
    if (imageModels.length && !imageModels.some((m) => m.code === imageModel)) setImageModel(imageModels[0]!.code);
    if (textModels.length && !textModels.some((m) => m.code === textModel)) setTextModel(textModels[0]!.code);
    if (videoModels.length && !videoModels.some((m) => m.code === videoModel)) setVideoModel(videoModels[0]!.code);
  }, [imageModel, imageModels, textModel, textModels, videoModel, videoModels]);

  useEffect(() => {
    const el = promptRef.current;
    if (!el) return;
    el.style.height = 'auto';
    el.style.height = `${Math.min(el.scrollHeight, 260)}px`;
    el.style.overflowY = el.scrollHeight > 260 ? 'auto' : 'hidden';
  }, [prompt, mode]);

  const history = useQuery({
    queryKey: ['gen.history', 'studio', token, historyPageSize],
    enabled: !!token,
    queryFn: () => genApi.history({ kind: 'media', page: 1, page_size: historyPageSize }),
  });

  const deleteHistory = useMutation({
    mutationFn: (scope: HistoryDeleteScope) => genApi.deleteHistory(scope),
    onSuccess: (res, scope) => {
      const label = scope === 'before_3d' ? '3天前作品' : scope === 'before_7d' ? '7天前作品' : '全部作品';
      toast.success(`已删除 ${res.deleted} 条${label}`);
      setTask(null);
      qc.invalidateQueries({ queryKey: ['gen.history'] });
    },
    onError: (e) => toast.error(e instanceof ApiError ? e.message : '删除失败'),
  });

  const createImage = useMutation({
    mutationFn: () => genApi.createImage({
      model: imageModel,
      prompt,
      count,
      ratio: imageRatio,
      ref_assets: attachments.map((item) => item.dataUrl),
      mode: attachments.length ? 'i2i' : 't2i',
      params: { resolution: imageResolution, quality: 'high' },
    }),
    onSuccess: (t) => handleTask(t),
    onError: (e) => toast.error(e instanceof ApiError ? e.message : '生成失败'),
  });

  const createVideo = useMutation({
    mutationFn: () => genApi.createVideo({ model: videoModel, prompt, duration, ratio: videoRatio, quality: 'hd', ref_assets: attachments.map((item) => item.dataUrl), mode: attachments.length ? 'i2v' : 't2v' }),
    onSuccess: (t) => handleTask(t),
    onError: (e) => toast.error(e instanceof ApiError ? e.message : '生成失败'),
  });

  const createText = useMutation({
    mutationFn: () => genApi.createText({ model: textModel, prompt, max_tokens: 1600, images: attachments.map((item) => item.dataUrl) }),
    onSuccess: async (res) => {
      setTextResult(res.content || '');
      toast.success('文字生成完成');
      await refreshMe();
      qc.invalidateQueries({ queryKey: ['gen.history'] });
    },
    onError: (e) => toast.error(e instanceof ApiError ? e.message : '生成失败'),
  });

  const inProgress = task && (task.status === 0 || task.status === 1);
  const resultItems = useMemo(() => {
    const visible = (item: GenerationTask) => (item.kind === 'image' || item.kind === 'video') && item.status !== 3;
    const current = task?.results?.length && visible(task) ? [task] : [];
    const rest = (history.data?.list ?? []).filter(visible);
    return [...current, ...rest].filter((item, idx, arr) => arr.findIndex((x) => x.task_id === item.task_id) === idx);
  }, [history.data?.list, task]);

  const expectedCost = mode === 'video'
    ? Math.round(((videoModels.find((m) => m.code === videoModel)?.cost ?? 20) * duration) / 6)
    : mode === 'text'
      ? '按实际 Token'
      : (imageModels.find((m) => m.code === imageModel)?.cost ?? 4) * count;
  const maxAttachments = mode === 'video' ? VIDEO_MAX_ATTACHMENTS : TEXT_MAX_ATTACHMENTS;

  const handleTask = (t: GenerationTask) => {
    setTask(t);
    startPolling(t.task_id);
    void refreshMe();
    qc.invalidateQueries({ queryKey: ['gen.history'] });
  };

  const startPolling = (taskId: string) => {
    if (pollRef.current) window.clearInterval(pollRef.current);
    pollRef.current = window.setInterval(async () => {
      try {
        const fresh = await genApi.getTask(taskId);
        setTask(fresh);
        if ([2, 3, 4].includes(fresh.status)) {
          if (pollRef.current) window.clearInterval(pollRef.current);
          pollRef.current = null;
          if (fresh.status === 2) toast.success('生成完成');
          else if (fresh.status === 3) toast.error(fresh.error || '生成失败');
          else toast.info('已退款');
          await refreshMe();
          qc.invalidateQueries({ queryKey: ['gen.history'] });
        }
      } catch {
        // keep polling quietly
      }
    }, mode === 'video' ? 2000 : 1500);
  };

  const submit = () => {
    if (!prompt.trim()) {
      toast.info('先描述你想创作的内容');
      return;
    }
    ensureLoggedIn(() => {
      if (mode === 'text') createText.mutate();
      else if (mode === 'video') createVideo.mutate();
      else createImage.mutate();
    }, '登录后即可开始创作');
  };

  const readFileAsDataURL = (file: File) => new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result || ''));
    reader.onerror = () => reject(reader.error || new Error('read file failed'));
    reader.readAsDataURL(file);
  });

  const handleAttachFiles = async (files: FileList | null) => {
    if (!files?.length) return;
    const imageFiles = Array.from(files).filter((file) => file.type.startsWith('image/'));
    if (!imageFiles.length) {
      toast.info('请选择图片文件');
      return;
    }
    const slots = Math.max(0, maxAttachments - attachments.length);
    if (slots <= 0) {
      toast.info(`最多上传 ${maxAttachments} 张参考图`);
      return;
    }
    const picked = imageFiles.slice(0, slots);
    try {
      const data = await Promise.all(picked.map(async (file) => ({
        id: `${file.name}-${file.size}-${file.lastModified}`,
        name: file.name,
        dataUrl: await readFileAsDataURL(file),
      })));
      setAttachments((prev) => [...prev, ...data]);
      if (imageFiles.length > slots) toast.info(`已保留前 ${maxAttachments} 张参考图`);
    } catch {
      toast.error('读取图片失败');
    } finally {
      if (fileInputRef.current) fileInputRef.current.value = '';
    }
  };

  return (
    <div className="mx-auto min-h-screen w-full max-w-[1500px] px-4 pb-12 pt-10 sm:px-8 lg:px-12">
      <section className="mx-auto max-w-[760px]">
        <div className="mb-6 flex items-center justify-between">
          <h1 className="text-[28px] font-medium tracking-normal text-neutral-950">{modeTitle(mode)}</h1>
          <ModeSwitch mode={mode} onChange={(next) => navigate(`/create/${next}`)} />
        </div>

        <div className="rounded-[28px] border border-neutral-200 bg-white p-4 shadow-[0_18px_55px_rgba(15,23,42,.10)]">
          <textarea
            ref={promptRef}
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            placeholder={mode === 'image' ? '描述新图片' : mode === 'video' ? '描述新视频' : '写下你想生成的文字内容'}
            className="studio-prompt min-h-[66px] w-full resize-none border-0 bg-transparent px-2 pt-1 text-[15px] font-normal leading-7 text-neutral-950 outline-none ring-0 placeholder:font-normal placeholder:text-neutral-400 focus:border-0 focus:outline-none focus:ring-0"
            maxLength={5000}
          />
          <div className="mt-2 flex items-center justify-between gap-3">
            <div className="flex min-w-0 flex-wrap items-center gap-2">
              <input
                ref={fileInputRef}
                type="file"
                accept="image/*"
                multiple
                className="hidden"
                onChange={(e) => void handleAttachFiles(e.target.files)}
              />
              <button
                className="grid h-8 w-8 place-items-center rounded-full text-neutral-600 hover:bg-neutral-100"
                title="上传参考图"
                type="button"
                onClick={() => fileInputRef.current?.click()}
              >
                <Paperclip size={18} />
              </button>
              <ComposerSelect
                value={mode === 'video' ? videoModel : mode === 'text' ? textModel : imageModel}
                onChange={(v) => mode === 'video' ? setVideoModel(v) : mode === 'text' ? setTextModel(v) : setImageModel(v)}
                options={(mode === 'video' ? videoModels : mode === 'text' ? textModels : imageModels).map((m) => ({ value: m.code, label: m.label }))}
                wide={mode !== 'image'}
              />
              {mode === 'image' && (
                <>
                  <ComposerSelect value={imageRatio} onChange={(v) => setImageRatio(v as typeof IMAGE_RATIOS[number])} options={IMAGE_RATIOS.map((r) => ({ value: r, label: r }))} />
                  <ComposerSelect value={imageResolution} onChange={(v) => setImageResolution(v as typeof IMAGE_RESOLUTIONS[number])} options={IMAGE_RESOLUTIONS.map((r) => ({ value: r, label: r }))} />
                  <ComposerSelect value={String(count)} onChange={(v) => setCount(Number(v))} options={[1, 2, 4].map((n) => ({ value: String(n), label: `${n}张` }))} />
                </>
              )}
              {mode === 'video' && (
                <>
                  <ComposerSelect value={videoRatio} onChange={(v) => setVideoRatio(v as typeof VIDEO_RATIOS[number])} options={VIDEO_RATIOS.map((r) => ({ value: r, label: r }))} />
                  <ComposerSelect value={String(duration)} onChange={(v) => setDuration(Number(v) as typeof VIDEO_DURATIONS[number])} options={VIDEO_DURATIONS.map((n) => ({ value: String(n), label: `${n}s` }))} />
                </>
              )}
            </div>
            <div className="flex items-center gap-2">
              <span className="hidden text-sm text-neutral-400 sm:inline">{typeof expectedCost === 'number' ? `${expectedCost} 点` : expectedCost}</span>
              <button className="grid h-8 w-8 place-items-center rounded-full text-neutral-600 hover:bg-neutral-100" title="语音输入" type="button">
                <Mic size={18} />
              </button>
              <button
                type="button"
                onClick={submit}
                disabled={!!inProgress || createImage.isPending || createVideo.isPending || createText.isPending}
                className="grid h-10 w-10 place-items-center rounded-full bg-neutral-950 text-white transition hover:bg-neutral-800 disabled:cursor-not-allowed disabled:bg-neutral-300"
                title="生成"
              >
                {inProgress || createImage.isPending || createVideo.isPending || createText.isPending ? <Loader2 size={18} className="animate-spin" /> : <ArrowUp size={19} />}
              </button>
            </div>
          </div>
          {attachments.length > 0 && (
            <div className="mt-3 flex flex-wrap gap-2">
              {attachments.map((item) => (
                <div key={item.id} className="group relative h-14 w-14 overflow-hidden rounded-[12px] bg-neutral-100">
                  <img src={item.dataUrl} alt={item.name} className="h-full w-full object-cover" />
                  <button
                    type="button"
                    onClick={() => setAttachments((prev) => prev.filter((x) => x.id !== item.id))}
                    className="absolute right-1 top-1 grid h-5 w-5 place-items-center rounded-full bg-black/60 text-white opacity-0 transition group-hover:opacity-100"
                    title="移除"
                  >
                    <X size={12} />
                  </button>
                </div>
              ))}
            </div>
          )}
        </div>
      </section>

      {mode === 'text' && textResult && (
        <section className="mx-auto mt-8 max-w-[760px] rounded-[24px] border border-neutral-200 bg-white p-5 text-[15px] leading-7 text-neutral-800 shadow-sm">
          <div className="mb-3 flex items-center justify-between text-sm text-neutral-400">
            <span>{textModels.find((m) => m.code === textModel)?.label ?? textModel}</span>
            <span>{textResult.length} 字</span>
          </div>
          <div className="whitespace-pre-wrap">{textResult}</div>
        </section>
      )}

      {mode === 'image' && (
        <section className="mx-auto mt-12 max-w-[760px]">
          <div className="mb-4 flex items-center justify-between">
            <h2 className="text-[20px] font-medium text-neutral-950">创建图片</h2>
            <div className="flex items-center gap-2 text-neutral-400">
              <button className="grid h-9 w-9 place-items-center rounded-full border border-neutral-200 hover:text-neutral-900"><ChevronLeft size={18} /></button>
              <button className="grid h-9 w-9 place-items-center rounded-full border border-neutral-200 hover:text-neutral-900"><ChevronRight size={18} /></button>
            </div>
          </div>
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-5">
            {SUGGESTIONS.map((item) => (
              <button
                key={item.title}
                type="button"
                onClick={() => setPrompt(item.prompt)}
                className="group relative aspect-[4/5] overflow-hidden rounded-[22px] text-left shadow-sm"
              >
                <img src={item.image} alt={item.title} className="absolute inset-0 h-full w-full object-cover transition duration-300 group-hover:scale-[1.03]" loading="lazy" />
                <div className="absolute inset-0 bg-gradient-to-t from-black/70 via-black/10 to-transparent" />
                <span className="absolute bottom-3 left-3 right-3 text-sm font-medium text-white">{item.title}</span>
              </button>
            ))}
          </div>
        </section>
      )}

      <section className="mt-14">
        <div className="mx-auto mb-4 flex max-w-[1500px] items-center justify-between gap-3 px-0">
          <h2 className="text-[20px] font-medium text-neutral-950">我的作品</h2>
          <div className="flex items-center gap-2">
            <ComposerSelect
              value={String(historyPageSize)}
              onChange={(v) => setHistoryPageSize(Number(v) as typeof HISTORY_PAGE_SIZES[number])}
              options={HISTORY_PAGE_SIZES.map((n) => ({ value: String(n), label: `${n}个` }))}
            />
            <HistoryActionMenu
              disabled={!token || deleteHistory.isPending}
              onDeleteBefore3Days={() => {
                if (window.confirm('确定删除3天前的作品记录吗？')) {
                  deleteHistory.mutate('before_3d');
                }
              }}
              onDeleteBefore7Days={() => {
                if (window.confirm('确定删除7天前的作品记录吗？')) {
                  deleteHistory.mutate('before_7d');
                }
              }}
              onDeleteAll={() => {
                if (window.confirm('确定删除全部作品记录吗？已完成、失败和退款记录都会从首页移除。')) {
                  deleteHistory.mutate('all');
                }
              }}
            />
          </div>
        </div>
        {resultItems.length === 0 ? (
          <div className="mx-auto grid max-w-[1500px] place-items-center rounded-[24px] border border-dashed border-neutral-200 py-14 text-neutral-400">
            <FileImage size={28} />
            <p className="mt-2 text-sm">{token ? '还没有作品，先生成一张图片吧' : '登录后会在这里显示你的作品'}</p>
          </div>
        ) : (
          <div
            className="mx-auto max-w-[1500px] columns-1 gap-3 sm:columns-2 lg:columns-3 xl:columns-4 2xl:columns-5"
            style={{ columnWidth: '220px' }}
          >
            {resultItems.map((item) => <WorkCard key={item.task_id} item={item} onOpen={setPreview} />)}
          </div>
        )}
      </section>
      {preview && <PreviewLightbox preview={preview} onClose={() => setPreview(null)} />}
    </div>
  );
}

function ModeSwitch({ mode, onChange }: { mode: StudioMode; onChange: (mode: StudioMode) => void }) {
  return (
    <div className="inline-flex rounded-full bg-neutral-100 p-1">
      {MODES.map((m) => {
        const Icon = m.icon;
        return (
          <button
            key={m.value}
            type="button"
            onClick={() => onChange(m.value)}
            className={clsx(
              'inline-flex h-8 items-center gap-1.5 rounded-full px-3 text-sm transition',
              mode === m.value ? 'bg-white text-neutral-950 shadow-sm' : 'text-neutral-600 hover:text-neutral-950',
            )}
          >
            <Icon size={15} />
            {m.label}
          </button>
        );
      })}
    </div>
  );
}

function ComposerSelect({ value, options, onChange, disabled, wide }: { value: string; options: { value: string; label: string }[]; onChange: (value: string) => void; disabled?: boolean; wide?: boolean }) {
  const [open, setOpen] = useState(false);
  const current = options.find((o) => o.value === value) ?? options[0];

  return (
    <div
      className="relative"
      onBlur={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setOpen(false);
      }}
    >
      <button
        type="button"
        disabled={disabled}
        onClick={() => setOpen((v) => !v)}
        className={clsx(
          'inline-flex h-8 items-center gap-1.5 rounded-full px-3 text-sm text-sky-500 outline-none transition',
          wide && 'min-w-[150px] justify-between',
          open ? 'bg-sky-50' : 'hover:bg-neutral-100',
          disabled && 'cursor-not-allowed text-neutral-400 hover:bg-transparent',
        )}
      >
        <span>{current?.label}</span>
        <ChevronDown size={15} className={clsx('transition', open && 'rotate-180')} />
      </button>

      {open && !disabled && (
        <div className={clsx('absolute left-0 top-10 z-30 overflow-hidden rounded-[18px] border border-neutral-200 bg-white p-1.5 shadow-[0_18px_50px_rgba(15,23,42,.14)]', wide ? 'min-w-[190px]' : 'min-w-[132px]')}>
          {options.map((o) => {
            const selected = o.value === value;
            return (
              <button
                key={o.value}
                type="button"
                onMouseDown={(e) => e.preventDefault()}
                onClick={() => {
                  onChange(o.value);
                  setOpen(false);
                }}
                className={clsx(
                  'flex h-10 w-full items-center justify-between gap-3 rounded-[12px] px-3 text-left text-sm transition',
                  selected ? 'bg-neutral-100 text-neutral-950' : 'text-neutral-600 hover:bg-neutral-50 hover:text-neutral-950',
                )}
              >
                <span>{o.label}</span>
                {selected && <Check size={16} />}
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}

function HistoryActionMenu({
  disabled,
  onDeleteBefore3Days,
  onDeleteBefore7Days,
  onDeleteAll,
}: {
  disabled?: boolean;
  onDeleteBefore3Days: () => void;
  onDeleteBefore7Days: () => void;
  onDeleteAll: () => void;
}) {
  const [open, setOpen] = useState(false);
  return (
    <div
      className="relative"
      onBlur={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setOpen(false);
      }}
    >
      <button
        type="button"
        disabled={disabled}
        onClick={() => setOpen((v) => !v)}
        className="inline-flex h-8 items-center gap-1.5 rounded-full px-3 text-sm text-neutral-600 outline-none transition hover:bg-neutral-100 disabled:cursor-not-allowed disabled:text-neutral-300"
      >
        <MoreHorizontal size={16} />
        管理
      </button>
      {open && !disabled && (
        <div className="absolute right-0 top-10 z-30 min-w-[150px] overflow-hidden rounded-[18px] border border-neutral-200 bg-white p-1.5 shadow-[0_18px_50px_rgba(15,23,42,.14)]">
          <button
            type="button"
            onMouseDown={(e) => e.preventDefault()}
            onClick={() => {
              setOpen(false);
              onDeleteBefore3Days();
            }}
            className="flex h-10 w-full items-center gap-2 rounded-[12px] px-3 text-left text-sm text-neutral-700 transition hover:bg-neutral-50"
          >
            <Trash2 size={15} />
            删除3天前
          </button>
          <button
            type="button"
            onMouseDown={(e) => e.preventDefault()}
            onClick={() => {
              setOpen(false);
              onDeleteBefore7Days();
            }}
            className="flex h-10 w-full items-center gap-2 rounded-[12px] px-3 text-left text-sm text-neutral-700 transition hover:bg-neutral-50"
          >
            <Trash2 size={15} />
            删除7天前
          </button>
          <button
            type="button"
            onMouseDown={(e) => e.preventDefault()}
            onClick={() => {
              setOpen(false);
              onDeleteAll();
            }}
            className="flex h-10 w-full items-center gap-2 rounded-[12px] px-3 text-left text-sm text-red-600 transition hover:bg-red-50"
          >
            <Trash2 size={15} />
            删除全部
          </button>
        </div>
      )}
    </div>
  );
}

function WorkCard({ item, onOpen }: { item: GenerationTask; onOpen: (preview: { url: string; type: 'image' | 'video'; title: string }) => void }) {
  const result = item.results?.[0];
  const thumb = result?.thumb_url;
  const original = result?.url;
  const [thumbFailed, setThumbFailed] = useState(false);
  const [loadedRatio, setLoadedRatio] = useState<string | null>(null);
  const isVideo = item.kind === 'video';
  const showThumb = !!thumb && !thumbFailed;
  const declaredRatio = result?.width && result?.height ? `${result.width} / ${result.height}` : '';
  const mediaRatio = loadedRatio || declaredRatio || (isVideo ? '16 / 9' : '1 / 1');
  const canOpen = item.status === 2 && !!original;
  const prompt = compactPrompt(item.prompt);
  const setRatioFromImage = (el: HTMLImageElement) => {
    if (el.naturalWidth > 0 && el.naturalHeight > 0) {
      setLoadedRatio(`${el.naturalWidth} / ${el.naturalHeight}`);
    }
  };
  const setRatioFromVideo = (el: HTMLVideoElement) => {
    if (el.videoWidth > 0 && el.videoHeight > 0) {
      setLoadedRatio(`${el.videoWidth} / ${el.videoHeight}`);
    }
  };

  return (
    <article className="mb-3 break-inside-avoid overflow-hidden rounded-[6px] bg-neutral-100">
      <button
        type="button"
        disabled={!canOpen}
        onClick={() => original && onOpen({ url: original, type: isVideo ? 'video' : 'image', title: item.model })}
        style={{ aspectRatio: mediaRatio }}
        className={clsx(
          'relative grid w-full place-items-center overflow-hidden text-neutral-400 transition-[height]',
          !original && item.status === 1 && 'bg-white',
          canOpen && 'group cursor-zoom-in',
        )}
      >
        {original ? (
          isVideo ? (
            showThumb ? (
              <img
                src={thumb}
                alt=""
                className="h-full w-full object-cover"
                loading="lazy"
                onLoad={(e) => setRatioFromImage(e.currentTarget)}
                onError={() => setThumbFailed(true)}
              />
            ) : (
              <video
                src={original}
                className="h-full w-full object-cover"
                muted
                playsInline
                preload="metadata"
                onLoadedMetadata={(e) => setRatioFromVideo(e.currentTarget)}
              />
            )
          ) : (
            <img src={original} alt="" className="h-full w-full object-cover" loading="lazy" onLoad={(e) => setRatioFromImage(e.currentTarget)} />
          )
        ) : item.status === 1 ? (
          <GeneratingDots />
        ) : (
          <div className="flex flex-col items-center gap-2 text-sm">
            <FileImage size={24} />
            <span>{statusText(item.status)}</span>
          </div>
        )}
        <div className="absolute left-2 top-2 rounded-full bg-black/55 px-2 py-0.5 text-xs text-white">{item.kind === 'video' ? '\u89c6\u9891' : '\u56fe\u7247'}</div>
        {canOpen && (
          <div className="absolute inset-0 grid place-items-center bg-black/0 opacity-0 transition group-hover:bg-black/20 group-hover:opacity-100">
            <span className="grid h-10 w-10 place-items-center rounded-full bg-white/90 text-neutral-950 shadow-sm">
              {isVideo ? <Play size={18} fill="currentColor" /> : <Maximize2 size={18} />}
            </span>
          </div>
        )}
      </button>
      <div className="flex items-center gap-1.5 px-2.5 py-2 text-xs text-neutral-500">
        <span className="shrink-0">{fmtRelative(item.created_at)}</span>
        {prompt && <span className="truncate text-neutral-600">{prompt}</span>}
      </div>
    </article>
  );
}

function compactPrompt(prompt?: string) {
  const text = String(prompt || '').replace(/\s+/g, ' ').trim();
  if (!text) return '';
  return text.length > 28 ? text.slice(0, 28) + '...' : text;
}

function GeneratingDots() {
  const [phraseIndex, setPhraseIndex] = useState(0);

  useEffect(() => {
    const timer = window.setInterval(() => {
      setPhraseIndex((idx) => (idx + 1) % GENERATING_PHRASES.length);
    }, 1800);
    return () => window.clearInterval(timer);
  }, []);

  return (
    <div className="generating-dots" aria-label="正在为您设计中">
      <div className="generating-dots__phrases">
        <span className="generating-dots__phrase generating-dots__phrase--active" key={phraseIndex}>
          {GENERATING_PHRASES[phraseIndex]}
        </span>
      </div>
    </div>
  );
}

function PreviewLightbox({ preview, onClose }: { preview: { url: string; type: 'image' | 'video'; title: string }; onClose: () => void }) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/75 p-4" onMouseDown={onClose}>
      <div className="relative max-h-[92vh] max-w-[92vw]" onMouseDown={(e) => e.stopPropagation()}>
        <button
          type="button"
          onClick={onClose}
          className="absolute right-3 top-3 z-10 grid h-9 w-9 place-items-center rounded-full bg-white/90 text-neutral-900 shadow-sm transition hover:bg-white"
          title="关闭"
        >
          <X size={18} />
        </button>
        {preview.type === 'video' ? (
          <video src={preview.url} controls autoPlay className="max-h-[92vh] max-w-[92vw] rounded-[12px] bg-black shadow-2xl" />
        ) : (
          <img src={preview.url} alt={preview.title} className="max-h-[92vh] max-w-[92vw] rounded-[12px] object-contain shadow-2xl" />
        )}
      </div>
    </div>
  );
}

function modeFromPath(pathname: string): StudioMode {
  if (pathname.includes('/create/video')) return 'video';
  if (pathname.includes('/create/text')) return 'text';
  return 'image';
}

function modeTitle(mode: StudioMode) {
  if (mode === 'video') return '视频';
  if (mode === 'text') return '文字';
  return '图片';
}

function statusText(status: number) {
  if (status === 2) return '已完成';
  if (status === 3) return '失败';
  if (status === 4) return '已退款';
  if (status === 1) return '生成中';
  return '排队中';
}

function modelsByKind(models: PublicModel[] | undefined, kind: PublicModel['kind'], fallback: SelectModel[]): SelectModel[] {
  const rows = (models ?? [])
    .filter((m) => m.enabled !== false && m.kind === kind && m.model_code)
    .map((m) => ({
      code: m.model_code,
      label: m.name || m.model_code,
      cost: typeof m.unit_points === 'number' ? m.unit_points / 100 : undefined,
      input: typeof m.input_unit_points === 'number' ? m.input_unit_points / 100 : undefined,
      output: typeof m.output_unit_points === 'number' ? m.output_unit_points / 100 : undefined,
    }));
  return rows.length ? rows : fallback;
}
