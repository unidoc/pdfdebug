/**
 * @file Right-hand detail panel. Shows the full object detail for the
 * selected tree node, with a contextual header label. A tab bar at the top
 * carries Object (per-selection), XREF (document-level xref table) and Plain
 * Text (document-level Latin-1 bytes).
 */
import { useState, useEffect, useCallback, useMemo, useRef, memo } from 'react';
import * as Tabs from '@radix-ui/react-tabs';
import { GetObjectDetail, GetContentStream, GetImageData, DescribeImage, SaveImageToFile, GetReverseRefs, GetFontView, GetSignatures, OpenFile, OpenFileDialog, CloseDocument } from '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js';
import { ContentStreamData, ImageData as PdfImageData } from '../../bindings/unidoc-pdf-debugger/internal/pdfcore/models.js';
import { useAppState, useAppDispatch } from '../hooks/useDocumentState';
import { extractErrorMessage } from '../lib/extractErrorMessage';
import { IMAGE_WARN_THRESHOLD_BYTES, IMAGE_SLOW_THRESHOLD_SECONDS } from '../lib/imageConstants';
import { formatBytes } from '../lib/formatBytes';
import {
  type ObjectDetailData,
  type ObjectFindContext,
  DictView,
  ArrayView,
  ScalarView,
} from './DetailShared';
import { useSpanFind, substringSpanMatcher, type FindSpan, type SpanMatch } from '../hooks/useSpanFind';
import { FindBar } from './FindBar';
import { ContentStreamViewer, type StreamViewMode } from './ContentStreamViewer';
import { ImagePreview } from './ImagePreview';
import { FontPreview, type FontDetailData } from './FontPreview';
import { FontRosterPreview, type FontResourceMapData } from './FontRosterPreview';
import { ReverseRefsSection, type ReverseRefEntry } from './ReverseRefsSection';
import { XRefTableView } from './XRefTableView';
import { PlainTextView } from './PlainTextView';
import { EmbeddedDataView } from './EmbeddedDataView';
import { DocumentMetadataView } from './DocumentMetadataView';
import { SignaturesView, type SignatureEntryData } from './SignaturesView';
import { ValidateView } from './ValidateView';
import { DiffView } from './DiffView';

/**
 * Matches indirect-object node IDs exactly (e.g. "obj:0:5"). Inline nodes
 * carry trailing dict/arr segments after the obj prefix and must be excluded
 * from the Referenced by section.
 */
const INDIRECT_NODE_RE = /^obj:\d+:\d+$/;

/**
 * Lowercase substring used to detect the index-unavailable sentinel coming
 * back from Wails error rejection. Matches the canonical
 * `ErrReverseRefIndexUnavailable` wording ("reverse-ref index unavailable").
 */
const REV_REFS_UNAVAILABLE_MARKER = 'reverse-ref index unavailable';

/** Maps PDF object type to a human-readable header label. */
const TYPE_LABEL_MAP: Record<string, string> = {
  dict: 'Properties',
  array: 'Array',
  stream: 'Content Stream',
  scalar: 'Value',
};


/** Decode-free pre-decode estimate returned by DescribeImage. */
interface ImageDescriptionData {
  width: number;
  height: number;
  colorSpace: string;
  estimatedBytes: number;
}

/** Renders an estimated byte count as a compact size (e.g. "192 MB"). */
/** Render-state for the iconHint='font' branch. Encodes the four possible
 *  outcomes of a GetFontView fetch: detail payload (render FontPreview),
 *  roster (render FontRosterPreview for the /Resources /Font map),
 *  fallback (render generic DictView when the dict is neither a Font dict nor
 *  a font roster), inline error message (render error string in the dict-view
 *  slot for real backend failures). */
type FontFetchState =
  | { kind: 'detail'; detail: FontDetailData }
  | { kind: 'roster'; roster: FontResourceMapData }
  | { kind: 'fallback' }
  | { kind: 'error'; message: string }
  | null;

/** Which of the seven DetailPanel tabs is currently active. 'embedded' is
 *  attachments/associated files and 'metadata' is Info + XMP; 'signatures' is
 *  shown only when signature fields exist; 'validate' runs the structural
 *  conformance checks; 'diff' is the side-by-side structural diff against a
 *  second PDF. */
type DetailView = 'object' | 'xref' | 'plaintext' | 'embedded' | 'metadata' | 'validate' | 'signatures' | 'diff';

/** Inner (un-memoized) detail panel that fetches and renders object detail. */
function DetailPanelInner() {
  const { tabs, activeTabId } = useAppState();
  const dispatch = useAppDispatch();
  const activeTab = tabs.find((t) => t.tabId === activeTabId);
  const selectedNodeId = activeTab?.selectedNodeId ?? null;
  const selectedNodeLabel = activeTab?.selectedNodeLabel ?? null;
  const selectedNodeRawKey = activeTab?.selectedNodeRawKey ?? null;
  const selectedNodeIconHint = activeTab?.selectedNodeIconHint ?? null;
  const navHistory = activeTab?.navHistory ?? [];
  const navHistoryIndex = activeTab?.navHistoryIndex ?? -1;
  const canGoBack = navHistoryIndex > 0;
  const canGoForward = navHistoryIndex < navHistory.length - 1;
  const isMac = useMemo(() => navigator.platform.startsWith('Mac'), []);

  const [detail, setDetail] = useState<ObjectDetailData | null>(null);
  const [detailTabId, setDetailTabId] = useState<string | null>(null);
  // The node id the current `detail` payload was fetched for. Find is gated on
  // this matching the live selection so stale detail is not searched mid-load.
  const [detailNodeId, setDetailNodeId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [contentStream, setContentStream] = useState<ContentStreamData | null>(null);
  const [contentStreamLoading, setContentStreamLoading] = useState(false);
  const [showContentStreamLoading, setShowContentStreamLoading] = useState(false);
  const [streamViewMode, setStreamViewMode] = useState<StreamViewMode>('formatted');
  const [imageData, setImageData] = useState<PdfImageData | null>(null);
  const [imageDescription, setImageDescription] = useState<ImageDescriptionData | null>(null);
  // True when the pre-decode estimate is above the warning threshold: the panel
  // announces the size and defers the decode until the user proceeds.
  const [imageConsent, setImageConsent] = useState(false);
  const [imageLoading, setImageLoading] = useState(false);
  const [imageElapsed, setImageElapsed] = useState(0);
  const [imageSaveError, setImageSaveError] = useState<string | null>(null);
  // Monotonic token guarding stale image requests across selection changes.
  const imageReqRef = useRef(0);
  // Tracks the currently selected node id so a save failure is dropped only when
  // the node actually changed, not when the same node re-decodes.
  const selectedImageNodeRef = useRef<string | null>(null);
  // Blocks a second full-resolution save render while one is already in flight.
  const savingRef = useRef(false);
  // Holds the in-flight GetImageData cancellable call so navigating away can
  // abort the backend decode (not just drop the result). Cancelling sends a
  // CancelCall to Go, which cancels the render context at its next checkpoint.
  const imageLoadPromiseRef = useRef<ReturnType<typeof GetImageData> | null>(null);
  const [fontState, setFontState] = useState<FontFetchState>(null);
  const [showFontLoading, setShowFontLoading] = useState(false);

  // Reverse-refs are fetched per-selection. The backend has the
  // document-level index so the call is O(1); no client cache is needed.
  // reverseRefsLoaded gates the section render until the fetch resolves so an
  // in-flight selection does not flash the orphan empty state for objects that
  // actually have inbound refs.
  const [reverseRefs, setReverseRefs] = useState<ReverseRefEntry[]>([]);
  const [reverseRefsUnavailable, setReverseRefsUnavailable] = useState(false);
  const [reverseRefsVisible, setReverseRefsVisible] = useState(false);
  const [reverseRefsLoaded, setReverseRefsLoaded] = useState(false);

  // per-document local state for the active DetailPanel tab.
  // Resets to 'object' on activeTabId change so a fresh document opens on the
  // per-selection view. Mirrors the streamViewMode pattern.
  const [detailView, setDetailView] = useState<DetailView>('object');
  // Entry count from the XREF tab, used in the "XREF (N)" tab label.
  const [xrefEntryCount, setXrefEntryCount] = useState<number | null>(null);
  // Embedded-file count from the Embedded tab, used in the "Embedded (N)" tab
  // label (mirrors the XREF count pattern).
  const [embeddedCount, setEmbeddedCount] = useState<number | null>(null);
  // The signature list drives the Signatures tab visibility. ONE
  // GetSignatures fetch per document tab, made on mount and cached here (no
  // refetch per tab switch); the tab is simply absent until the fetch
  // resolves with >= 1 signature field. null = not yet resolved.
  const [signatures, setSignatures] = useState<SignatureEntryData[] | null>(null);
  // The second (comparison) document's tab ID for the Diff tab, and
  // any picker error. Reset on document switch so the diff never carries over a
  // stale comparison from a previously-active tab.
  const [diffRightTabId, setDiffRightTabId] = useState<string | null>(null);
  const [diffError, setDiffError] = useState<string | null>(null);
  // Tracks whether the panel is still mounted so a diff file dialog that
  // resolves after unmount closes its backend document instead of leaking it
  // (the diffRightTabId cleanup effect only registers when state is set, which
  // never happens if the panel unmounted mid-dialog). See handlePickDiffFile.
  const mountedRef = useRef(true);
  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);
  // Tracks the latest active document tab so an in-flight diff-file pick that
  // resolves AFTER the user switches documents is discarded (its backend
  // document closed) rather than bound to the wrong baseline. The reset effect
  // clears diffRightTabId on switch, so a late-landing pick would otherwise set
  // it against a document that is no longer the left-hand side of the diff.
  const activeTabIdRef = useRef(activeTabId);
  activeTabIdRef.current = activeTabId;

  useEffect(() => {
    setDetailView('object');
    setXrefEntryCount(null);
    setEmbeddedCount(null);
    setSignatures(null);
    setDiffRightTabId(null);
    setDiffError(null);
  }, [activeTabId]);

  // The comparison document is opened backend-only (not an app tab),
  // so close it when it is replaced, cleared on document switch, or the panel
  // unmounts - otherwise every diff leaks a parsed document in the Go backend.
  useEffect(() => {
    if (!diffRightTabId) return;
    return () => {
      CloseDocument(diffRightTabId).catch(() => {
        /* best-effort cleanup; a failed close must not break the UI */
      });
    };
  }, [diffRightTabId]);

  // One signature fetch per document tab. The result is passed
  // down to SignaturesView via the data prop so the view never issues a
  // second fetch. A fetch failure hides the tab (empty list) and logs.
  useEffect(() => {
    if (!activeTabId) return;
    let cancelled = false;
    GetSignatures(activeTabId)
      .then((result: unknown) => {
        if (cancelled) return;
        setSignatures((Array.isArray(result) ? result : []) as SignatureEntryData[]);
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        setSignatures([]);
        // eslint-disable-next-line no-console
        console.warn('GetSignatures failed:', err);
      });
    return () => { cancelled = true; };
  }, [activeTabId]);

  useEffect(() => {
    if (!activeTabId || !selectedNodeId) {
      setDetail(null);
      setError(null);
      setContentStream(null);
      setContentStreamLoading(false);
      setShowContentStreamLoading(false);
      setImageData(null);
      setImageLoading(false);
      setImageConsent(false);
      setFontState(null);
      setShowFontLoading(false);
      return;
    }
    // Keep previous detail/contentStream visible until the new fetch resolves
    // to avoid a flash of empty or error state during tab switches.
    setError(null);
    // Stale-fetch guard: discard response if selection changed before resolve
    let cancelled = false;
    GetObjectDetail(activeTabId, selectedNodeId)
      .then((result: unknown) => {
        if (!cancelled) {
          setDetail(result as ObjectDetailData);
          setDetailTabId(activeTabId);
          setDetailNodeId(selectedNodeId);
        }
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(extractErrorMessage(err));
        }
      });
    return () => { cancelled = true; };
  }, [activeTabId, selectedNodeId]);

  // Fetch content stream when detail resolves to a non-image stream node.
  // Uses detailTabId (the tab that produced this detail) to avoid a
  // mismatched fetch when activeTabId changes before detail updates.
  useEffect(() => {
    if (!detail || !detailTabId) return;
    if (detail.type !== 'stream' || selectedNodeIconHint === 'image') {
      setContentStream(null);
      setContentStreamLoading(false);
      return;
    }
    setContentStreamLoading(true);
    let cancelled = false;
    GetContentStream(detailTabId, detail.nodeId)
      .then((result: unknown) => {
        if (!cancelled) {
          setContentStream(result as ContentStreamData);
          setContentStreamLoading(false);
        }
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setContentStream(new ContentStreamData({ nodeId: detail.nodeId, raw: '', tokenized: [], formatted: [], error: extractErrorMessage(err) }));
          setContentStreamLoading(false);
        }
      });
    return () => { cancelled = true; };
  }, [detail, detailTabId, selectedNodeIconHint]);

  // Debounce content stream loading indicator by 200ms
  useEffect(() => {
    if (!contentStreamLoading) {
      setShowContentStreamLoading(false);
      return;
    }
    const timer = setTimeout(() => setShowContentStreamLoading(true), 200);
    return () => clearTimeout(timer);
  }, [contentStreamLoading]);

  // Decode an image and show its preview. Guarded by imageReqRef so a result
  // from a superseded selection is dropped.
  const decodeImage = useCallback((tabId: string, nodeId: string) => {
    const token = ++imageReqRef.current;
    setImageConsent(false);
    setImageData(null);
    setImageSaveError(null);
    setImageLoading(true);
    const promise = GetImageData(tabId, nodeId);
    imageLoadPromiseRef.current = promise;
    promise
      .then((result: unknown) => {
        if (imageReqRef.current !== token) return;
        imageLoadPromiseRef.current = null;
        setImageData(result as PdfImageData);
        setImageLoading(false);
      })
      .catch((err: unknown) => {
        if (imageReqRef.current !== token) return;
        imageLoadPromiseRef.current = null;
        // A cancel (navigating away) is not an error: clear loading and show
        // nothing. Any other failure renders as an error preview.
        const msg = extractErrorMessage(err);
        if ((err instanceof Error && err.name === 'CancelError') || /cancel/i.test(msg)) {
          setImageLoading(false);
          return;
        }
        setImageData(new PdfImageData({ nodeId, kind: 'error', error: msg }));
        setImageLoading(false);
      });
  }, []);

  // On an image node, describe it first (decode-free) so an expensive decode can
  // be announced before it runs; a within-threshold image decodes immediately.
  // imageLoading is set from selection so the loading indicator's elapsed timer
  // starts before the describe resolves.
  useEffect(() => {
    if (selectedNodeIconHint !== 'image' || !detail || detail.type !== 'stream' || !detailTabId) {
      imageReqRef.current++;
      setImageData(null);
      setImageDescription(null);
      setImageConsent(false);
      setImageLoading(false);
      setImageSaveError(null);
      return;
    }
    const tabId = detailTabId;
    const nodeId = detail.nodeId;
    const token = ++imageReqRef.current;
    setImageData(null);
    setImageDescription(null);
    setImageConsent(false);
    setImageSaveError(null);
    setImageLoading(true);
    DescribeImage(tabId, nodeId)
      .then((result: unknown) => {
        if (imageReqRef.current !== token) return;
        const desc = result as ImageDescriptionData;
        setImageDescription(desc);
        if (desc && desc.estimatedBytes > IMAGE_WARN_THRESHOLD_BYTES) {
          setImageConsent(true);
          setImageLoading(false);
          return;
        }
        decodeImage(tabId, nodeId);
      })
      .catch(() => {
        // Describe is a best-effort pre-warning; fall back to decoding directly.
        if (imageReqRef.current === token) decodeImage(tabId, nodeId);
      });
    return () => {
      // Cancel the in-flight decode when the selection changes or the panel
      // unmounts, so the backend stops decoding instead of running to
      // completion. The imageReqRef bump still drops any result that races in.
      imageLoadPromiseRef.current?.cancel();
      imageLoadPromiseRef.current = null;
    };
  }, [detail, detailTabId, selectedNodeIconHint, decodeImage]);

  // Elapsed-seconds counter while an image node is selected and still resolving
  // (describe or decode in flight), driving the loading indicator's escalation.
  // Keyed on the synchronous selection rather than the async imageLoading so the
  // counter starts before the detail/describe round-trips resolve.
  useEffect(() => {
    if (selectedNodeIconHint !== 'image' || imageData || imageConsent) {
      setImageElapsed(0);
      return;
    }
    setImageElapsed(0);
    const id = setInterval(() => setImageElapsed((e) => e + 1), 1000);
    return () => clearInterval(id);
  }, [selectedNodeIconHint, imageData, imageConsent]);

  // Mirror the selected node id into a ref so an async save-failure handler can
  // tell whether the selection moved while its save was in flight.
  useEffect(() => {
    selectedImageNodeRef.current = detail?.nodeId ?? null;
  }, [detail]);

  const handleProceedImage = useCallback(() => {
    if (!detailTabId || !detail) return;
    decodeImage(detailTabId, detail.nodeId);
  }, [detail, detailTabId, decodeImage]);

  const handleSaveImage = useCallback(async () => {
    if (!detailTabId || !detail) return;
    if (savingRef.current) return;
    savingRef.current = true;
    const nodeIdAtSave = detail.nodeId;
    setImageSaveError(null);
    try {
      // Sanitize the node-derived base: object refs like "obj:0:7" carry colons,
      // which are invalid in filenames on Windows.
      const base = (detail.objectRef || detail.nodeId || 'image').split(' ')[0].replace(/[^\w.-]/g, '-');
      await SaveImageToFile(detailTabId, detail.nodeId, `image-${base}.png`);
    } catch (err) {
      // Drop a failure only when the selected node changed while the save was in
      // flight; a same-node re-decode (Load after Save) must still show the error.
      if (selectedImageNodeRef.current !== nodeIdAtSave) return;
      setImageSaveError(extractErrorMessage(err));
    } finally {
      savingRef.current = false;
    }
  }, [detail, detailTabId]);

  // Fetch the unified FontView when detail resolves to a dict node
  // tagged iconHint='font'. The backend disambiguates the three outcomes
  // ("detail" / "roster" / "neither") in one call so the binding layer never
  // logs ERR on the iconHint='font' false positive. .catch fires only on
  // genuine backend errors (unknown tab, malformed PDF, pdfcpu panics).
  useEffect(() => {
    if (selectedNodeIconHint !== 'font' || !detail || detail.type !== 'dict' || !detailTabId) {
      setFontState(null);
      return;
    }
    setFontState(null);
    let cancelled = false;
    GetFontView(detailTabId, detail.nodeId)
      .then((result: unknown) => {
        if (cancelled) return;
        const view = result as { kind: string; detail: FontDetailData | null; roster: FontResourceMapData | null };
        if (view?.kind === 'detail' && view.detail) {
          setFontState({ kind: 'detail', detail: view.detail });
        } else if (view?.kind === 'roster' && view.roster) {
          setFontState({ kind: 'roster', roster: view.roster });
        } else {
          setFontState({ kind: 'fallback' });
        }
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        setFontState({ kind: 'error', message: extractErrorMessage(err) });
      });
    return () => { cancelled = true; };
  }, [detail, detailTabId, selectedNodeIconHint]);

  // 200ms-debounced loading indicator timer for the font fetch. Keyed on
  // selectedNodeId + iconHint so it starts as soon as the user clicks a
  // font node -- before detail resolves. This avoids a microtask-ordering
  // gap where the timer would otherwise be unscheduled until detail settled
  // (visible only under tests that use sync vi.advanceTimersByTime; real
  // users always see consistent 200ms behaviour). The JSX condition
  // `!fontState && showFontLoading` hides the indicator once fontState
  // resolves, so a stale timer that fires after fetch completion has no
  // visible effect.
  useEffect(() => {
    if (selectedNodeIconHint !== 'font' || !selectedNodeId) {
      setShowFontLoading(false);
      return;
    }
    setShowFontLoading(false);
    const timer = setTimeout(() => setShowFontLoading(true), 200);
    return () => clearTimeout(timer);
  }, [selectedNodeId, selectedNodeIconHint]);

  // Fetch reverse refs for indirect-object selections only. The
  // catalog (nodeId='root') is also treated as indirect because in real PDFs
  // it lives in the indirect-object graph and the section must render the
  // "Document root..." empty state for it. Inline-value nodes never
  // mount the section. Stale-fetch guard matches existing patterns.
  useEffect(() => {
    const isIndirect = !!selectedNodeId &&
      (INDIRECT_NODE_RE.test(selectedNodeId) || selectedNodeId === 'root');
    if (!activeTabId || !isIndirect) {
      setReverseRefs([]);
      setReverseRefsUnavailable(false);
      setReverseRefsVisible(false);
      setReverseRefsLoaded(false);
      return;
    }
    // Reset transient state on every selection change so a previous selection's
    // banner doesn't bleed into the new selection while its fetch is in flight.
    // reverseRefsLoaded stays false until the fetch resolves so the section
    // does not flash the orphan empty state for non-orphan objects.
    setReverseRefs([]);
    setReverseRefsUnavailable(false);
    setReverseRefsVisible(true);
    setReverseRefsLoaded(false);
    let cancelled = false;
    GetReverseRefs(activeTabId, selectedNodeId)
      .then((result: unknown) => {
        if (cancelled) return;
        const list = (Array.isArray(result) ? result : []) as ReverseRefEntry[];
        setReverseRefs(list);
        setReverseRefsUnavailable(false);
        setReverseRefsLoaded(true);
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        const msg = extractErrorMessage(err);
        if (msg.toLowerCase().includes(REV_REFS_UNAVAILABLE_MARKER)) {
          // Failure mode: surface the unavailable banner.
          setReverseRefs([]);
          setReverseRefsUnavailable(true);
          setReverseRefsLoaded(true);
        } else {
          // Any other rejection: hide the section silently and log.
          setReverseRefs([]);
          setReverseRefsUnavailable(false);
          setReverseRefsVisible(false);
          setReverseRefsLoaded(false);
          // eslint-disable-next-line no-console
          console.warn('GetReverseRefs failed:', err);
        }
      });
    return () => { cancelled = true; };
  }, [activeTabId, selectedNodeId]);

  // Keyboard shortcuts: Cmd+[ / Ctrl+[ for back, Cmd+] / Ctrl+] for forward
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      const mod = e.metaKey || e.ctrlKey;
      if (!mod) return;
      if (e.key === '[') {
        e.preventDefault();
        if (canGoBack) dispatch({ type: 'NAVIGATE_BACK' });
      } else if (e.key === ']') {
        e.preventDefault();
        if (canGoForward) dispatch({ type: 'NAVIGATE_FORWARD' });
      }
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [canGoBack, canGoForward, dispatch]);

  /** Navigate the tree to the referenced PDF object. */
  const handleReferenceClick = useCallback((refTarget: string) => {
    if (refTarget) {
      dispatch({ type: 'NAVIGATE_TO_REF', payload: { targetNodeId: refTarget } });
    }
  }, [dispatch]);

  /**
   * XREF row click handler. Switches the active tab to Object BEFORE
   * dispatching navigation so React batches both updates in one render and
   * the user never sees a flash of "XREF active + new selection".
   */
  const handleXRefNavigate = useCallback((nodeId: string) => {
    setDetailView('object');
    dispatch({ type: 'NAVIGATE_TO_REF', payload: { targetNodeId: nodeId } });
  }, [dispatch]);

  /**
   * Embedded "Reveal in tree" handler: switches to the Object tab BEFORE
   * dispatching navigation so the user lands on the /EmbeddedFile stream object
   * in one render (mirrors handleXRefNavigate).
   */
  const handleEmbeddedNavigate = useCallback((nodeId: string) => {
    setDetailView('object');
    dispatch({ type: 'NAVIGATE_TO_REF', payload: { targetNodeId: nodeId } });
  }, [dispatch]);

  /**
   * Signatures "Reveal in tree" handler: switches to the Object tab BEFORE
   * dispatching navigation so the user lands on the /V signature dict (or the
   * field node fallback) in one render.
   */
  const handleSignaturesNavigate = useCallback((nodeId: string) => {
    setDetailView('object');
    dispatch({ type: 'NAVIGATE_TO_REF', payload: { targetNodeId: nodeId } });
  }, [dispatch]);

  /**
   * Validate "jump to object" handler: switches to the Object tab BEFORE
   * dispatching navigation so the user lands on the offending object in one
   * render (mirrors handleSignaturesNavigate).
   */
  const handleValidateNavigate = useCallback((nodeId: string) => {
    setDetailView('object');
    dispatch({ type: 'NAVIGATE_TO_REF', payload: { targetNodeId: nodeId } });
  }, [dispatch]);

  /**
   * Pick a second PDF to diff against the active document. Opens a
   * native file dialog, loads the chosen file into the shared inspector as a new
   * tab, and stores its tab ID as the diff's right-hand side. A cancelled dialog
   * (empty selection) is a no-op.
   */
  const handlePickDiffFile = useCallback(() => {
    setDiffError(null);
    const startTab = activeTabId;
    OpenFileDialog()
      .then((paths: unknown) => {
        const list = Array.isArray(paths) ? (paths as string[]) : [];
        if (list.length === 0) return; // cancelled
        return OpenFile(list[0]).then((info: unknown) => {
          const di = info as { tabId?: string } | null;
          if (!di?.tabId) return;
          // If the panel unmounted OR the active document changed while the
          // dialog/parse was in flight, the diffRightTabId cleanup effect does
          // not cover this tab (the reset effect already cleared it), so close
          // the just-opened backend document here rather than leak it or bind
          // the diff against the wrong (now-inactive) baseline.
          if (mountedRef.current && activeTabIdRef.current === startTab) {
            setDiffRightTabId(di.tabId);
          } else {
            CloseDocument(di.tabId).catch(() => {});
          }
        });
      })
      .catch((err: unknown) => setDiffError(extractErrorMessage(err)));
  }, [activeTabId]);

  // --- Object-tab find (Model B: scoped to the active tab). Derives a linear
  // match source from the rendered object text (keys + values) and reuses the
  // shared find machinery. The window listener is gated on the Object tab being
  // active, so a query here never reaches another tab. ---
  const findCaseSensitive = activeTab?.findCaseSensitive ?? false;
  // Object find only has a highlight surface in the DictView / ArrayView /
  // ScalarView render paths. Font detail/roster previews, image previews and
  // content-stream views render their own components that ignore `find`, so the
  // bar must not open or report matches there (it would show a count with no
  // visible marks). Mirrors the render branches in the Object pane below.
  const objectFindable = useMemo(() => {
    if (!detail) return false;
    // The previous selection's detail is kept on screen while the next one
    // loads. Do not treat that stale payload as searchable: a query entered
    // during the load window would otherwise carry into the newly loaded object.
    if (detailTabId !== activeTabId || detailNodeId !== selectedNodeId) return false;
    if (detail.type === 'dict') {
      if (selectedNodeIconHint === 'font') return fontState?.kind === 'fallback';
      return true;
    }
    if (detail.type === 'array') return true;
    if (detail.type === 'scalar') return !!detail.scalarValue;
    return false;
  }, [detail, detailTabId, detailNodeId, activeTabId, selectedNodeId, selectedNodeIconHint, fontState]);

  const objectSpans = useMemo<FindSpan[]>(() => {
    const spans: FindSpan[] = [];
    if (!objectFindable || !detail) return spans;
    if (detail.type === 'dict' && detail.properties) {
      detail.properties.forEach((p, i) => {
        spans.push({ id: `prop:${i}:key`, text: p.key });
        spans.push({ id: `prop:${i}:value`, text: p.value.display });
      });
    } else if (detail.type === 'array' && detail.elements) {
      detail.elements.forEach((e, i) => spans.push({ id: `elem:${i}`, text: e.display }));
    } else if (detail.type === 'scalar' && detail.scalarValue) {
      spans.push({ id: 'scalar', text: detail.scalarValue.display });
    }
    return spans;
  }, [detail, objectFindable]);

  const objectFind = useSpanFind({
    // Fold the selected node into the reset key so navigating to a different
    // object starts find fresh (no query carried over to the new object).
    resetKey: `${activeTabId ?? ''}::${selectedNodeId ?? ''}`,
    spans: objectSpans,
    matcher: substringSpanMatcher,
    caseSensitive: findCaseSensitive,
    active: detailView === 'object',
    ready: objectFindable,
    barTestId: 'object-find-bar',
  });

  const objectMatchesBySpan = useMemo(() => {
    const map = new Map<string, SpanMatch[]>();
    for (const m of objectFind.matches) {
      const arr = map.get(m.spanId);
      if (arr) arr.push(m);
      else map.set(m.spanId, [m]);
    }
    return map;
  }, [objectFind.matches]);

  const objectFindContext: ObjectFindContext = {
    matchesBySpan: objectMatchesBySpan,
    activeMatch: objectFind.activeMatch,
    prefix: 'object-find',
  };

  const handleObjectCaseToggle = useCallback(() => {
    if (!activeTabId) return;
    dispatch({ type: 'SET_FIND_CASE_SENSITIVE', payload: { tabId: activeTabId, value: !findCaseSensitive } });
  }, [dispatch, activeTabId, findCaseSensitive]);

  // Scroll the active object match into view. The Object views are not
  // virtualized, so the active mark is always in the DOM once matches render.
  // Keyed on the active match alone (not `open`) so navigating between matches
  // scrolls, while opening or closing the bar -- which does not move the active
  // match -- never re-scrolls. Closing clears the query, so no active mark
  // remains and the querySelector no-ops.
  const objectContentRef = useRef<HTMLDivElement | null>(null);
  useEffect(() => {
    const el = objectContentRef.current?.querySelector('[data-testid="object-find-active-match"]');
    if (el && typeof el.scrollIntoView === 'function') el.scrollIntoView({ block: 'nearest' });
  }, [objectFind.activeMatch]);

  // Close the Object find bar and move focus back onto the Object content, so a
  // keyboard user does not land on document.body when the input unmounts
  // (mirrors the Plain Text close). tabIndex=-1 is set lazily so the container
  // can accept programmatic focus without entering the tab order.
  const handleObjectFindClose = useCallback(() => {
    objectFind.closeBar();
    const el = objectContentRef.current;
    if (el) {
      if (!el.hasAttribute('tabindex')) el.setAttribute('tabindex', '-1');
      el.focus({ preventScroll: true });
    }
  }, [objectFind]);

  // The Signatures tab exists only when the document has >= 1 signature
  // field (hidden while unresolved or empty -- a deliberate departure from
  // the always-visible tabs, avoiding a permanently empty tab).
  const showSignaturesTab = (signatures?.length ?? 0) > 0;

  // FontPreview is active when iconHint='font', detail is a dict, and the
  // fetch resolved to a detail payload (not fallback / error). The header
  // contract is "Font - <BaseFont>" (with BaseFont falling back to "" -> just
  // "Font"); preempts the generic TYPE_LABEL_MAP "Properties" entry.
  const fontActive = selectedNodeIconHint === 'font'
    && detail?.type === 'dict'
    && fontState?.kind === 'detail';
  let typeLabel: string | null = null;
  let contextSuffix: string | null = selectedNodeRawKey || selectedNodeLabel;
  if (detail) {
    if (fontActive && fontState?.kind === 'detail') {
      typeLabel = 'Font';
      contextSuffix = fontState.detail.baseFont || null;
    } else if (selectedNodeIconHint === 'image') {
      typeLabel = 'Image Preview';
    } else {
      typeLabel = TYPE_LABEL_MAP[detail.type] ?? 'Details';
    }
  }
  const headerLabel = typeLabel && contextSuffix
    ? `${typeLabel} - ${contextSuffix}`
    : typeLabel;

  const xrefLabel = xrefEntryCount !== null ? `XREF (${xrefEntryCount})` : 'XREF';
  const embeddedLabel = embeddedCount !== null ? `Embedded (${embeddedCount})` : 'Embedded';

  const tabTriggerClass =
    'px-3 py-1 text-xs text-text-secondary border-b-2 border-transparent hover:text-text hover:bg-surface-hover focus:outline-none focus-visible:ring-2 focus-visible:ring-border-focus cursor-pointer data-[state=active]:text-text data-[state=active]:border-b-border-focus';

  return (
    <div className="h-full flex flex-col" data-testid="detail-panel">
      <Tabs.Root
        value={detailView}
        onValueChange={(v) => setDetailView(v as DetailView)}
        activationMode="manual"
        className="h-full flex flex-col"
      >
        <Tabs.List
          aria-label="Detail view"
          className="flex border-b border-border bg-surface flex-shrink-0"
          data-testid="detail-tab-list"
          // Synchronous arrow-key focus movement. Radix's roving-focus group
          // also handles arrows, but it uses setTimeout(focusFirst), which is
          // async. Synthetic fireEvent.keyDown in tests cannot observe that
          // deferred focus, so we move focus synchronously here. The Radix
          // handler still runs after this (composed via React event bubbling)
          // and is a no-op when the target already has focus.
          onKeyDown={(e) => {
            if (e.key !== 'ArrowRight' && e.key !== 'ArrowLeft') return;
            const target = e.target as HTMLElement;
            if (target.getAttribute('role') !== 'tab') return;
            const triggers = Array.from(
              e.currentTarget.querySelectorAll<HTMLElement>('[role="tab"]')
            );
            const idx = triggers.indexOf(target);
            if (idx === -1) return;
            const dir = e.key === 'ArrowRight' ? 1 : -1;
            const next = triggers[(idx + dir + triggers.length) % triggers.length];
            if (next) next.focus();
          }}
        >
          {/* asChild + explicit onClick so synthetic click events
            (fireEvent.click in tests, and screen-reader virtual-cursor clicks)
            still activate the tab. Radix's own Tabs.Trigger only wires onMouseDown. */}
          <Tabs.Trigger value="object" asChild>
            <button
              type="button"
              className={tabTriggerClass}
              data-testid="detail-tab-object"
              onClick={() => setDetailView('object')}
            >
              Object
            </button>
          </Tabs.Trigger>
          <Tabs.Trigger value="xref" asChild>
            <button
              type="button"
              className={tabTriggerClass}
              data-testid="detail-tab-xref"
              onClick={() => setDetailView('xref')}
            >
              {xrefLabel}
            </button>
          </Tabs.Trigger>
          <Tabs.Trigger value="plaintext" asChild>
            <button
              type="button"
              className={tabTriggerClass}
              data-testid="detail-tab-plaintext"
              onClick={() => setDetailView('plaintext')}
            >
              Plain Text
            </button>
          </Tabs.Trigger>
          <Tabs.Trigger value="embedded" asChild>
            <button
              type="button"
              className={tabTriggerClass}
              data-testid="detail-tab-embedded"
              onClick={() => setDetailView('embedded')}
            >
              {embeddedLabel}
            </button>
          </Tabs.Trigger>
          <Tabs.Trigger value="metadata" asChild>
            <button
              type="button"
              className={tabTriggerClass}
              data-testid="detail-tab-metadata"
              onClick={() => setDetailView('metadata')}
            >
              Metadata
            </button>
          </Tabs.Trigger>
          <Tabs.Trigger value="validate" asChild>
            <button
              type="button"
              className={tabTriggerClass}
              data-testid="detail-tab-validate"
              onClick={() => setDetailView('validate')}
            >
              Validate
            </button>
          </Tabs.Trigger>
          <Tabs.Trigger value="diff" asChild>
            <button
              type="button"
              className={tabTriggerClass}
              data-testid="detail-tab-diff"
              onClick={() => setDetailView('diff')}
            >
              Diff
            </button>
          </Tabs.Trigger>
          {showSignaturesTab && (
            <Tabs.Trigger value="signatures" asChild>
              <button
                type="button"
                className={tabTriggerClass}
                data-testid="detail-tab-signatures"
                onClick={() => setDetailView('signatures')}
              >
                Signatures
              </button>
            </Tabs.Trigger>
          )}
        </Tabs.List>

        <Tabs.Content
          value="object"
          forceMount
          className="flex-1 min-h-0 data-[state=inactive]:hidden"
          data-testid="detail-pane-object"
        >
          <div className="h-full" aria-live="polite" data-testid="detail-panel-content" ref={objectContentRef}>
            {!selectedNodeId && (
              <div
                className="h-full flex items-center justify-center text-text-muted text-sm"
                data-testid="detail-panel-empty"
              >
                Select a node in the tree to view details
              </div>
            )}
            {selectedNodeId && error && (
              <div
                className="p-3 text-error text-sm"
                data-testid="detail-panel-error"
              >
                {error}
              </div>
            )}
            {selectedNodeId && detail && (
              <div className="h-full flex flex-col">
                <div
                  className="px-3 py-1.5 border-b border-border flex-shrink-0 flex items-center justify-between"
                  data-testid="detail-panel-header"
                >
                  <div className="flex items-center gap-2">
                    <div className="flex items-center gap-0.5">
                      <button
                        onClick={() => dispatch({ type: 'NAVIGATE_BACK' })}
                        disabled={!canGoBack}
                        title={`Back (${isMac ? 'Cmd+[' : 'Ctrl+['})`}
                        className={`p-0.5 rounded text-sm ${canGoBack ? 'text-text-secondary hover:bg-surface-hover cursor-pointer' : 'text-text-muted/40 cursor-not-allowed'}`}
                        data-testid="nav-back-button"
                        aria-label="Navigate back"
                      >
                        <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"><path d="M10 3L5 8l5 5"/></svg>
                      </button>
                      <button
                        onClick={() => dispatch({ type: 'NAVIGATE_FORWARD' })}
                        disabled={!canGoForward}
                        title={`Forward (${isMac ? 'Cmd+]' : 'Ctrl+]'})`}
                        className={`p-0.5 rounded text-sm ${canGoForward ? 'text-text-secondary hover:bg-surface-hover cursor-pointer' : 'text-text-muted/40 cursor-not-allowed'}`}
                        data-testid="nav-forward-button"
                        aria-label="Navigate forward"
                      >
                        <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"><path d="M6 3l5 5-5 5"/></svg>
                      </button>
                    </div>
                    <span className="text-sm font-medium text-text-secondary">{headerLabel}</span>
                  </div>
                  {detail.objectRef && (
                    <span className="text-xs text-text-muted font-mono">{detail.objectRef}</span>
                  )}
                </div>
                {objectFind.open && (
                  <FindBar
                    idPrefix="object-find"
                    label="Find in object"
                    matchCount={objectFind.matches.length}
                    activeIndex={objectFind.activeIndex}
                    query={objectFind.query}
                    caseSensitive={findCaseSensitive}
                    wrapped={objectFind.wrapped}
                    nonLatin1={false}
                    focusVersion={objectFind.focusVersion}
                    onQueryChange={objectFind.setQuery}
                    onNext={objectFind.next}
                    onPrev={objectFind.prev}
                    onCaseToggle={handleObjectCaseToggle}
                    onClose={handleObjectFindClose}
                  />
                )}
                {detail.type === 'dict' && selectedNodeIconHint === 'font' && (
                  <>
                    {fontState?.kind === 'detail' && (
                      <FontPreview
                        detail={fontState.detail}
                        onReferenceClick={handleReferenceClick}
                      />
                    )}
                    {fontState?.kind === 'roster' && (
                      <FontRosterPreview
                        roster={fontState.roster}
                        onReferenceClick={handleReferenceClick}
                      />
                    )}
                    {fontState?.kind === 'fallback' && (
                      <DictView properties={detail.properties} onReferenceClick={handleReferenceClick} find={objectFindContext} />
                    )}
                    {fontState?.kind === 'error' && (
                      <div className="p-3 text-error text-sm" data-testid="font-preview-error">
                        {fontState.message}
                      </div>
                    )}
                    {!fontState && showFontLoading && (
                      <div className="p-3 text-text-muted text-sm" data-testid="font-loading">
                        Loading font...
                      </div>
                    )}
                  </>
                )}
                {detail.type === 'dict' && selectedNodeIconHint !== 'font' && (
                  <DictView properties={detail.properties} onReferenceClick={handleReferenceClick} find={objectFindContext} />
                )}
                {detail.type === 'array' && <ArrayView elements={detail.elements} onReferenceClick={handleReferenceClick} find={objectFindContext} />}
                {detail.type === 'scalar' && (detail.scalarValue
                  ? <ScalarView value={detail.scalarValue} onReferenceClick={handleReferenceClick} find={objectFindContext} />
                  : <div className="text-text-muted text-sm p-3">No value</div>
                )}
                {detail.type === 'stream' && selectedNodeIconHint === 'image' && (
                  <>
                    {imageConsent && imageDescription && (
                      <div className="p-3 text-sm text-text-secondary" data-testid="image-preview-consent">
                        <div className="mb-2">
                          {imageDescription.width} x {imageDescription.height} {imageDescription.colorSpace || 'image'}, about {formatBytes(imageDescription.estimatedBytes, 0)} decoded. Loading it decodes a large image and the document is busy while it loads.
                        </div>
                        <div className="flex items-center gap-2">
                          <button
                            type="button"
                            className="px-2 py-1 text-xs rounded border border-border text-text-secondary hover:bg-surface-hover"
                            data-testid="image-preview-proceed"
                            onClick={handleProceedImage}
                          >
                            Load image
                          </button>
                          <button
                            type="button"
                            className="px-2 py-1 text-xs rounded border border-border text-text-secondary hover:bg-surface-hover"
                            data-testid="image-preview-save"
                            onClick={handleSaveImage}
                          >
                            Save image...
                          </button>
                        </div>
                        {imageSaveError && (
                          <div className="mt-2 text-error text-xs" data-testid="image-preview-save-error">
                            {imageSaveError}
                          </div>
                        )}
                      </div>
                    )}
                    {imageData && imageData.kind === 'ceiling-refusal' && (
                      <div className="p-3 text-error text-sm" data-testid="image-preview-finding">
                        {imageData.error}
                      </div>
                    )}
                    {imageData && imageData.kind !== 'ceiling-refusal' && (
                      <ImagePreview
                        base64={imageData.base64}
                        mimeType={imageData.mimeType}
                        width={imageData.width}
                        height={imageData.height}
                        colorSpace={imageData.colorSpace}
                        bitsPerComponent={imageData.bitsPerComponent}
                        filter={imageData.filter}
                        thumbWidth={imageData.thumbWidth}
                        thumbHeight={imageData.thumbHeight}
                        warning={imageData.warning}
                        error={imageData.error}
                        onSave={handleSaveImage}
                        saveError={imageSaveError ?? undefined}
                      />
                    )}
                    {/* Shown immediately (no debounce, unlike the content-stream
                        indicator): image loads carry a two-step describe+decode,
                        and the elapsed-seconds counter communicates duration. */}
                    {imageLoading && !imageData && !imageConsent && (
                      <div className="p-3 text-text-muted text-sm" data-testid="image-loading">
                        {imageElapsed >= IMAGE_SLOW_THRESHOLD_SECONDS
                          ? `Still decoding - taking longer than expected. The document is busy while this image loads (${imageElapsed}s).`
                          : `Loading image... (${imageElapsed}s)`}
                      </div>
                    )}
                  </>
                )}
                {detail.type === 'stream' && selectedNodeIconHint !== 'image' && (
                  <>
                    {contentStream && (
                      <ContentStreamViewer
                        raw={contentStream.raw}
                        formatted={contentStream.formatted}
                        error={contentStream.error}
                        viewMode={streamViewMode}
                        onViewModeChange={setStreamViewMode}
                      />
                    )}
                    {showContentStreamLoading && !contentStream && (
                      <div className="p-3 text-text-muted text-sm" data-testid="content-stream-loading">
                        Decoding stream...
                      </div>
                    )}
                  </>
                )}
                {reverseRefsVisible && reverseRefsLoaded && selectedNodeId && (
                  <ReverseRefsSection
                    key={selectedNodeId}
                    entries={reverseRefs}
                    selectedIconHint={selectedNodeIconHint}
                    indexUnavailable={reverseRefsUnavailable}
                  />
                )}
              </div>
            )}
          </div>
        </Tabs.Content>

        <Tabs.Content
          value="xref"
          forceMount
          className="flex-1 min-h-0 data-[state=inactive]:hidden"
          data-testid="detail-pane-xref"
        >
          <XRefTableView
            tabId={activeTabId ?? ''}
            active={detailView === 'xref'}
            onNavigate={handleXRefNavigate}
            onLoaded={setXrefEntryCount}
            findCaseSensitive={findCaseSensitive}
            onFindCaseToggle={handleObjectCaseToggle}
          />
        </Tabs.Content>

        <Tabs.Content
          value="plaintext"
          forceMount
          className="flex-1 min-h-0 data-[state=inactive]:hidden"
          data-testid="detail-pane-plaintext"
        >
          <PlainTextView
            tabId={activeTabId ?? ''}
            active={detailView === 'plaintext'}
          />
        </Tabs.Content>

        <Tabs.Content
          value="embedded"
          forceMount
          className="flex-1 min-h-0 data-[state=inactive]:hidden"
          data-testid="detail-pane-embedded"
        >
          <EmbeddedDataView
            tabId={activeTabId ?? ''}
            active={detailView === 'embedded'}
            onNavigate={handleEmbeddedNavigate}
            onLoaded={setEmbeddedCount}
          />
        </Tabs.Content>

        <Tabs.Content
          value="metadata"
          forceMount
          className="flex-1 min-h-0 data-[state=inactive]:hidden"
          data-testid="detail-pane-metadata"
        >
          <DocumentMetadataView
            tabId={activeTabId ?? ''}
            active={detailView === 'metadata'}
          />
        </Tabs.Content>

        <Tabs.Content
          value="validate"
          forceMount
          className="flex-1 min-h-0 data-[state=inactive]:hidden"
          data-testid="detail-pane-validate"
        >
          <ValidateView
            tabId={activeTabId ?? ''}
            active={detailView === 'validate'}
            onNavigate={handleValidateNavigate}
          />
        </Tabs.Content>

        <Tabs.Content
          value="diff"
          forceMount
          className="flex-1 min-h-0 data-[state=inactive]:hidden"
          data-testid="detail-pane-diff"
        >
          {!diffRightTabId ? (
            <div
              className="p-4 text-sm text-text-muted flex flex-col items-start gap-2"
              data-testid="diff-pick"
            >
              <p>Compare this document against another PDF (structural, object-level diff - not a byte or pixel diff).</p>
              <button
                type="button"
                data-testid="diff-pick-file"
                className="px-2 py-1 text-xs rounded border border-border text-text-secondary hover:bg-surface-hover cursor-pointer"
                onClick={handlePickDiffFile}
                disabled={!activeTabId}
              >
                Compare with another PDF...
              </button>
              {diffError && (
                <div className="text-error" data-testid="diff-pick-error">
                  {diffError}
                </div>
              )}
            </div>
          ) : (
            <DiffView
              leftTabId={activeTabId ?? ''}
              rightTabId={diffRightTabId}
              active={detailView === 'diff'}
            />
          )}
        </Tabs.Content>

        {showSignaturesTab && (
          <Tabs.Content
            value="signatures"
            className="flex-1 min-h-0 data-[state=inactive]:hidden"
            data-testid="detail-pane-signatures"
          >
            <SignaturesView
              tabId={activeTabId ?? ''}
              active={detailView === 'signatures'}
              onNavigate={handleSignaturesNavigate}
              data={signatures}
            />
          </Tabs.Content>
        )}
      </Tabs.Root>
    </div>
  );
}

/**
 * Memoized detail panel. Re-renders only when props change (none currently),
 * relying on internal hooks for state updates.
 */
export const DetailPanel = memo(DetailPanelInner);
