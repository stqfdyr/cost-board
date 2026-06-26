import { FormEvent, useEffect, useMemo, useRef, useState } from "react";
import {
  closestCenter,
  DndContext,
  DragEndEvent,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
} from "@dnd-kit/core";
import {
  arrayMove,
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { Login, getToken, clearToken } from "./Login";

type BillingMonths = 1 | 3 | 6 | 12;

type CostItem = {
  id: string;
  name: string;
  category: string;
  amount: number;
  currency: string;
  billingMonths: BillingMonths;
  enabled: boolean;
  note?: string;
  order: number;
};

type DraftItem = {
  id?: string;
  name: string;
  category: string;
  amount: string;
  currency: string;
  billingMonths: string;
  enabled: boolean;
  note: string;
};

type EditorSession = {
  editingId: string | null;
  initialDraft: DraftItem;
};

type FxSnapshot = {
  date: string;
  fetchedAt: string;
  rates: Record<string, number>;
  source: string;
};

type FxState = {
  snapshot: FxSnapshot | null;
  status: "idle" | "loading" | "success" | "error";
  error: string | null;
  usingCache: boolean;
};

type GroupedItems = {
  category: string;
  totalCny: number;
  itemCount: number;
  rows: Array<
    CostItem & {
      annualOriginal: number;
      annualCny: number | null;
      rateToCny: number | null;
    }
  >;
};

type GroupRow = GroupedItems["rows"][number];

const STORAGE_KEY = "cost-board:items";
const FX_CACHE_KEY = "cost-board:fx-cache";
const FX_ENDPOINT = "https://api.frankfurter.dev/v2/rates";
const API_ITEMS_URL = "/api/items";
const API_LOGOUT_URL = "/api/logout";
const API_SAVE_DEBOUNCE_MS = 400;

const billingOptions: Array<{ months: BillingMonths; label: string }> = [
  { months: 1, label: "月付" },
  { months: 3, label: "季付" },
  { months: 6, label: "半年付" },
  { months: 12, label: "年付" },
];

const commonCurrencies: Array<{ code: string; label: string }> = [
  { code: "CNY", label: "人民币" },
  { code: "USD", label: "美元" },
  { code: "EUR", label: "欧元" },
  { code: "GBP", label: "英镑" },
  { code: "HKD", label: "港币" },
  { code: "JPY", label: "日元" },
  { code: "SGD", label: "新加坡元" },
  { code: "CAD", label: "加拿大元" },
  { code: "AUD", label: "澳大利亚元" },
  { code: "KRW", label: "韩元" },
];

const initialItems: CostItem[] = [
  {
    id: "dmit",
    name: "DMIT",
    category: "服务器服务",
    amount: 39.9,
    currency: "USD",
    billingMonths: 12,
    enabled: true,
    order: 0,
  },
  {
    id: "claw",
    name: "Claw",
    category: "服务器服务",
    amount: 12.6,
    currency: "USD",
    billingMonths: 12,
    enabled: true,
    order: 1,
  },
  {
    id: "geelinx",
    name: "GeeLinx",
    category: "服务器服务",
    amount: 23.33,
    currency: "EUR",
    billingMonths: 12,
    enabled: true,
    order: 2,
  },
  {
    id: "ccs",
    name: "CCS",
    category: "服务器服务",
    amount: 15.6,
    currency: "USD",
    billingMonths: 12,
    enabled: true,
    order: 3,
  },
  {
    id: "fox-cloud",
    name: "狐蒂云",
    category: "服务器服务",
    amount: 37,
    currency: "CNY",
    billingMonths: 12,
    enabled: true,
    order: 4,
  },
  {
    id: "domain",
    name: "域名",
    category: "服务器服务",
    amount: 2,
    currency: "USD",
    billingMonths: 12,
    enabled: true,
    order: 5,
  },
  {
    id: "apple-music",
    name: "app music",
    category: "付费服务",
    amount: 11,
    currency: "CNY",
    billingMonths: 1,
    enabled: true,
    order: 6,
  },
  {
    id: "uhd",
    name: "uhd",
    category: "付费服务",
    amount: 10,
    currency: "USD",
    billingMonths: 12,
    enabled: true,
    order: 7,
  },
];

const emptyDraft: DraftItem = {
  name: "",
  category: "",
  amount: "",
  currency: "CNY",
  billingMonths: "12",
  enabled: true,
  note: "",
};

const cnyCurrency = new Intl.NumberFormat("zh-CN", {
  style: "currency",
  currency: "CNY",
  maximumFractionDigits: 2,
});

function App() {
  const [authed, setAuthed] = useState(() => getToken() !== null);
  const [showLogin, setShowLogin] = useState(false);

  if (showLogin) {
    return (
      <Login
        onSuccess={() => {
          setAuthed(true);
          setShowLogin(false);
        }}
        onCancel={() => setShowLogin(false)}
      />
    );
  }

  return (
    <Board
      authed={authed}
      onLogin={() => setShowLogin(true)}
      onLogout={() => { clearToken(); setAuthed(false); }}
    />
  );
}

function Board({ authed, onLogin, onLogout }: { authed: boolean; onLogin: () => void; onLogout: () => void }) {
  const [items, setItems] = useState<CostItem[]>(() => loadItems());
  const [fxState, setFxState] = useState<FxState>(() => ({
    snapshot: loadFxCache(),
    status: "idle",
    error: null,
    usingCache: false,
  }));
  const [editorSession, setEditorSession] = useState<EditorSession | null>(null);
  const [justRefreshed, setJustRefreshed] = useState(false);
  const [syncState, setSyncState] = useState<"idle" | "offline">("idle");
  const refreshRequestIdRef = useRef(0);
  const refreshAbortRef = useRef<AbortController | null>(null);
  const refreshFeedbackTimerRef = useRef<number | null>(null);
  const itemsRef = useRef(items);
  itemsRef.current = items;
  const skipApiSaveRef = useRef(false);
  const apiReadyRef = useRef(false);
  const saveTimerRef = useRef<number | null>(null);
  const syncStateRef = useRef(syncState);
  syncStateRef.current = syncState;
  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: {
        distance: 6,
      },
    }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    }),
  );

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      const result = await fetchItemsFromApi();
      if (cancelled) return;
      if (result === null) {
        setSyncState("offline");
        apiReadyRef.current = true;
        return;
      }
      if (result.initialized) {
        skipApiSaveRef.current = true;
        setItems(result.items);
        window.localStorage.setItem(STORAGE_KEY, JSON.stringify(result.items));
        setSyncState("idle");
        apiReadyRef.current = true;
      } else {
        apiReadyRef.current = true;
        const seeded = await saveItemsToApi(itemsRef.current);
        if (!cancelled) setSyncState(seeded ? "idle" : "offline");
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(items));
    if (skipApiSaveRef.current) {
      skipApiSaveRef.current = false;
      return;
    }
    if (!apiReadyRef.current) {
      return;
    }
    if (saveTimerRef.current !== null) {
      window.clearTimeout(saveTimerRef.current);
    }
    saveTimerRef.current = window.setTimeout(() => {
      saveTimerRef.current = null;
      void saveItemsToApi(items).then((ok) => {
        if (!ok) {
          setSyncState("offline");
        } else if (syncStateRef.current === "offline") {
          setSyncState("idle");
        }
      });
    }, API_SAVE_DEBOUNCE_MS);
    return () => {
      if (saveTimerRef.current !== null) {
        window.clearTimeout(saveTimerRef.current);
        saveTimerRef.current = null;
      }
    };
  }, [items]);

  useEffect(() => {
    return () => {
      refreshAbortRef.current?.abort();
      if (refreshFeedbackTimerRef.current !== null) {
        window.clearTimeout(refreshFeedbackTimerRef.current);
      }
    };
  }, []);

  const activeItems = useMemo(
    () => items.filter((item) => item.enabled),
    [items],
  );

  const categories = useMemo(
    () => Array.from(new Set(items.map((item) => item.category))),
    [items],
  );

  const usedCurrencies = useMemo(() => {
    return Array.from(
      new Set(
        activeItems
          .map((item) => item.currency.toUpperCase())
          .filter((currency) => currency !== "CNY"),
      ),
    ).sort();
  }, [activeItems]);

  useEffect(() => {
    void refreshRates(usedCurrencies, false);
  }, [usedCurrencies.join(",")]);

  const groupedItems = useMemo<GroupedItems[]>(() => {
    const groups = new Map<string, GroupedItems>();

    for (const item of activeItems) {
      const annualOriginal = annualize(item.amount, item.billingMonths);
      const rateToCny = getRateToCny(item.currency, fxState.snapshot);
      const annualCny = rateToCny === null ? null : annualOriginal * rateToCny;
      const existing = groups.get(item.category) ?? {
        category: item.category,
        totalCny: 0,
        itemCount: 0,
        rows: [],
      };

      existing.rows.push({
        ...item,
        annualOriginal,
        annualCny,
        rateToCny,
      });
      existing.itemCount += 1;
      if (annualCny !== null) {
        existing.totalCny += annualCny;
      }
      groups.set(item.category, existing);
    }

    return Array.from(groups.values())
      .map((group) => ({
        ...group,
        rows: [...group.rows].sort((left, right) => left.order - right.order),
      }))
      .sort((left, right) => right.totalCny - left.totalCny);
  }, [activeItems, fxState.snapshot]);

  const summary = useMemo(() => {
    let totalAnnualCny = 0;
    const unavailableCurrencies = new Set<string>();

    for (const group of groupedItems) {
      totalAnnualCny += group.totalCny;

      for (const row of group.rows) {
        if (row.annualCny === null) {
          unavailableCurrencies.add(row.currency.toUpperCase());
        }
      }
    }

    return {
      totalAnnualCny,
      monthlyAverage: totalAnnualCny / 12,
      unavailableCurrencies: Array.from(unavailableCurrencies),
      disabledCount: items.length - activeItems.length,
      currencyCount: new Set(
        activeItems.map((item) => item.currency.toUpperCase()),
      ).size,
    };
  }, [activeItems, groupedItems, items.length]);
  const lastFetchedAt = formatTimestamp(fxState.snapshot?.fetchedAt);

  async function handleLogout() {
    const token = getToken();
    if (token) {
      try {
        await fetch(API_LOGOUT_URL, {
          method: "POST",
          headers: { Authorization: `Bearer ${token}` },
        });
      } catch {
        // ignore network errors on logout
      }
    }
    onLogout();
  }

  function openCreateEditor() {
    const defaultCategory = categories.length > 0 ? categories[0] : "";
    setEditorSession({
      editingId: null,
      initialDraft: { ...emptyDraft, category: defaultCategory },
    });
  }

  function openEditEditor(item: CostItem) {
    setEditorSession({
      editingId: item.id,
      initialDraft: {
        id: item.id,
        name: item.name,
        category: item.category,
        amount: String(item.amount),
        currency: item.currency,
        billingMonths: String(item.billingMonths),
        enabled: item.enabled,
        note: item.note ?? "",
      },
    });
  }

  function closeEditor() {
    setEditorSession(null);
  }

  function handleSubmitDraft(draft: DraftItem, editingId: string | null) {
    const normalizedCurrency = draft.currency.trim().toUpperCase();
    const normalizedCategory = draft.category.trim();
    const normalizedName = draft.name.trim();
    const amount = Number(draft.amount);
    const billingMonths = Number(draft.billingMonths) as BillingMonths;
    const previousItem = editingId
      ? items.find((item) => item.id === editingId) ?? null
      : null;
    const categoryChanged = previousItem
      ? previousItem.category !== normalizedCategory
      : false;

    if (!normalizedName || !normalizedCategory || !normalizedCurrency) {
      return;
    }

    if (!Number.isFinite(amount) || amount <= 0) {
      return;
    }

    if (![1, 3, 6, 12].includes(billingMonths)) {
      return;
    }

    const nextItem: CostItem = {
      id: editingId ?? makeId(),
      name: normalizedName,
      category: normalizedCategory,
      amount,
      currency: normalizedCurrency,
      billingMonths,
      enabled: draft.enabled,
      note: draft.note.trim() || undefined,
      order:
        editingId && previousItem && !categoryChanged
          ? previousItem.order
          : getNextOrder(items, normalizedCategory),
    };

    setItems((currentItems) => {
      if (!editingId) {
        return normalizeCategoryOrders([...currentItems, nextItem]);
      }

      return normalizeCategoryOrders(
        currentItems.map((item) =>
          item.id === editingId ? nextItem : item,
        ),
      );
    });

    closeEditor();
  }

  function toggleItemEnabled(id: string) {
    setItems((currentItems) =>
      normalizeCategoryOrders(
        currentItems.map((item) =>
          item.id === id ? { ...item, enabled: !item.enabled } : item,
        ),
      ),
    );
  }

  function removeItem(id: string) {
    setItems((currentItems) =>
      normalizeCategoryOrders(
        currentItems.filter((item) => item.id !== id),
      ),
    );
  }

  function clearDisabledItems() {
    setItems((currentItems) =>
      normalizeCategoryOrders(
        currentItems.filter((item) => item.enabled),
      ),
    );
  }

  function handleDragEnd(event: DragEndEvent) {
    const activeId = String(event.active.id);
    const overId = event.over ? String(event.over.id) : null;

    if (!overId || activeId === overId) {
      return;
    }

    setItems((currentItems) => {
      const activeItem = currentItems.find((item) => item.id === activeId);
      const overItem = currentItems.find((item) => item.id === overId);

      if (!activeItem || !overItem || activeItem.category !== overItem.category) {
        return currentItems;
      }

      const groupItems = currentItems
        .filter((item) => item.enabled && item.category === activeItem.category)
        .sort((left, right) => left.order - right.order);

      const activeIndex = groupItems.findIndex((item) => item.id === activeId);
      const overIndex = groupItems.findIndex((item) => item.id === overId);

      if (activeIndex < 0 || overIndex < 0) {
        return currentItems;
      }

      const reordered = arrayMove(groupItems, activeIndex, overIndex).map((item, index) => ({
        ...item,
        order: index,
      }));

      const reorderedMap = new Map(reordered.map((item) => [item.id, item.order]));

      return normalizeCategoryOrders(
        currentItems.map((item) => {
          if (item.category !== activeItem.category || !item.enabled) {
            return item;
          }

          const nextOrder = reorderedMap.get(item.id);
          return nextOrder === undefined ? item : { ...item, order: nextOrder };
        }),
      );
    });
  }

  function showRefreshFeedback() {
    setJustRefreshed(true);

    if (refreshFeedbackTimerRef.current !== null) {
      window.clearTimeout(refreshFeedbackTimerRef.current);
    }

    refreshFeedbackTimerRef.current = window.setTimeout(() => {
      setJustRefreshed(false);
      refreshFeedbackTimerRef.current = null;
    }, 2000);
  }

  async function refreshRates(
    currencies: string[],
    forceLoadingState: boolean,
  ) {
    refreshRequestIdRef.current += 1;
    const requestId = refreshRequestIdRef.current;

    refreshAbortRef.current?.abort();
    refreshAbortRef.current = null;

    if (currencies.length === 0) {
      setFxState({
        snapshot: {
          date: new Date().toISOString().slice(0, 10),
          fetchedAt: new Date().toISOString(),
          rates: { CNY: 1 },
          source: "local",
        },
        status: "success",
        error: null,
        usingCache: false,
      });
      if (forceLoadingState) {
        showRefreshFeedback();
      }
      return;
    }

    setFxState((current) => ({
      ...current,
      status: forceLoadingState ? "loading" : current.status,
      error: null,
      usingCache: false,
    }));

    const abortController = new AbortController();
    refreshAbortRef.current = abortController;
    const quotes = Array.from(new Set([...currencies, "CNY"])).join(",");

    try {
      const response = await fetch(`${FX_ENDPOINT}?base=EUR&quotes=${quotes}`, {
        signal: abortController.signal,
      });
      if (!response.ok) {
        throw new Error(`汇率接口返回 ${response.status}`);
      }

      const payload = (await response.json()) as Array<{
        date: string;
        base: string;
        quote: string;
        rate: number;
      }>;

      const date = payload[0]?.date;
      if (!date) {
        throw new Error("汇率数据为空");
      }

      const rates = payload.reduce<Record<string, number>>((result, entry) => {
        result[entry.quote] = entry.rate;
        return result;
      }, {});

      if (!rates.CNY) {
        throw new Error("汇率响应缺少 CNY");
      }

      if (requestId !== refreshRequestIdRef.current) {
        return;
      }

      const snapshot: FxSnapshot = {
        date,
        fetchedAt: new Date().toISOString(),
        rates,
        source: "Frankfurter",
      };

      window.localStorage.setItem(FX_CACHE_KEY, JSON.stringify(snapshot));
      setFxState({
        snapshot,
        status: "success",
        error: null,
        usingCache: false,
      });
      if (refreshAbortRef.current === abortController) {
        refreshAbortRef.current = null;
      }
      if (forceLoadingState) {
        showRefreshFeedback();
      }
    } catch (error) {
      if (
        abortController.signal.aborted ||
        (error instanceof DOMException && error.name === "AbortError") ||
        requestId !== refreshRequestIdRef.current
      ) {
        return;
      }

      const message =
        error instanceof Error ? error.message : "无法获取最新汇率";
      const cachedSnapshot = loadFxCache();

      if (refreshAbortRef.current === abortController) {
        refreshAbortRef.current = null;
      }

      if (cachedSnapshot) {
        setFxState({
          snapshot: cachedSnapshot,
          status: "error",
          error: message,
          usingCache: true,
        });
        return;
      }

      setFxState({
        snapshot: null,
        status: "error",
        error: message,
        usingCache: false,
      });
    }
  }

  return (
    <div className="app-shell">
      <main className="page">
        <header className="page-toolbar">
          <div className="page-title-block">
            <div className="toolbar-meta">
              <span className="meta-pill">
                <span className="meta-pill-label">汇率日期</span>
                <strong>{fxState.snapshot?.date ?? "未获取"}</strong>
              </span>
              <span className="meta-pill">
                <span className="meta-pill-label">最近获取</span>
                <strong>{lastFetchedAt}</strong>
              </span>
              <span className="meta-pill">
                <span className="meta-pill-label">分类</span>
                <strong>{groupedItems.length} 个</strong>
              </span>
              <span className="meta-pill">
                <span className="meta-pill-label">币种</span>
                <strong>{summary.currencyCount} 个</strong>
              </span>
            </div>
          </div>

          <div className="toolbar-actions">
            {authed ? (
              <>
                <button className="primary-button" type="button" onClick={openCreateEditor}>
                  添加项目
                </button>
                <button
                  className="secondary-button"
                  type="button"
                  onClick={() => {
                    void refreshRates(usedCurrencies, true);
                  }}
                >
                  {fxState.status === "loading" ? "刷新中..." : justRefreshed ? "已更新 ✓" : "刷新汇率"}
                </button>
                <button
                  className="secondary-button logout-button"
                  type="button"
                  onClick={handleLogout}
                >
                  退出
                </button>
              </>
            ) : (
              <button className="primary-button" type="button" onClick={onLogin}>
                登录管理
              </button>
            )}
          </div>
        </header>

        <section className="metrics-grid">
          <article className="metric-card metric-card-highlight">
            <span className="metric-label">年度总支出</span>
            <strong className="metric-value">{cnyCurrency.format(summary.totalAnnualCny)}</strong>
            <span className="metric-hint">RMB / 年</span>
          </article>

          <article className="metric-card">
            <span className="metric-label">月均折合</span>
            <strong className="metric-value">{cnyCurrency.format(summary.monthlyAverage)}</strong>
            <span className="metric-hint">RMB / 月</span>
          </article>

          <article className="metric-card">
            <span className="metric-label">活跃项目</span>
            <strong className="metric-value">{activeItems.length}</strong>
            <span className="metric-hint">
              {authed ? `已停用 ${summary.disabledCount} 项` : ""}
            </span>
          </article>

          <article className="metric-card">
            <span className="metric-label">汇率状态</span>
            <strong className="metric-value metric-value-small">
              {fxState.snapshot?.date ?? "未获取"}
            </strong>
            <span className="metric-hint">
              {fxState.usingCache
                ? "缓存汇率"
                : `来源 ${fxState.snapshot?.source ?? "Frankfurter"}`}
            </span>
          </article>
        </section>

        {(syncState === "offline" || fxState.error || summary.unavailableCurrencies.length > 0) && (
          <section className="status-strip">
            {syncState === "offline" && (
              <p>
                无法连接服务器，当前仅显示本地缓存，变更将在恢复连接后同步。
              </p>
            )}
            {fxState.error && (
              <p>
                汇率更新失败：{fxState.error}
              </p>
            )}
            {summary.unavailableCurrencies.length > 0 && (
              <p>
                以下币种暂未完成折算：
                {summary.unavailableCurrencies.join("、")}
              </p>
            )}
          </section>
        )}

        <DndContext
          collisionDetection={closestCenter}
          onDragEnd={handleDragEnd}
          sensors={authed ? sensors : []}
        >
          <section className="groups-column">
            {groupedItems.map((group) => (
              <article className="group-card" key={group.category}>
                <header className="group-header">
                  <div>
                    <p className="group-title">{group.category}</p>
                    <p className="group-subtitle">{group.itemCount} 个项目</p>
                  </div>
                  <div className="group-total-block">
                    <span className="group-total-label">年支出</span>
                    <strong className="group-total">
                      {cnyCurrency.format(group.totalCny)}
                    </strong>
                  </div>
                </header>

                <div className={"group-list-header" + (authed ? "" : " group-list-header-readonly")}>
                    <span>项目</span>
                    <span>原价</span>
                    <span>折合 RMB/年</span>
                    {authed && <span>操作</span>}
                  </div>

                <SortableContext
                  items={group.rows.map((item) => item.id)}
                  strategy={verticalListSortingStrategy}
                >
                  <div className="group-list">
                    {group.rows.map((item) => (
                      <SortableCostRow
                        item={item}
                        key={item.id}
                        readOnly={!authed}
                        onEdit={() => openEditEditor(item)}
                        onToggle={() => toggleItemEnabled(item.id)}
                      />
                    ))}
                  </div>
                </SortableContext>
              </article>
            ))}
          </section>
        </DndContext>

        {authed && summary.disabledCount > 0 && (
          <section className="disabled-card">
            <div className="disabled-card-header">
              <div>
                <p className="group-title">已停用项目</p>
                <span className="group-subtitle">{summary.disabledCount} 项</span>
              </div>
              <button
                className="danger-button"
                type="button"
                onClick={() => {
                  if (
                    window.confirm(
                      `确认清空全部 ${summary.disabledCount} 个已停用项目？此操作不可撤销。`,
                    )
                  ) {
                    clearDisabledItems();
                  }
                }}
              >
                清空
              </button>
            </div>
            <div className="disabled-list">
              {items
                .filter((item) => !item.enabled)
                .map((item) => (
                  <div className="disabled-row" key={item.id}>
                    <span>{item.name}</span>
                    <div className="disabled-row-actions">
                      <button
                        className="row-button"
                        type="button"
                        onClick={() => toggleItemEnabled(item.id)}
                      >
                        启用
                      </button>
                      <button
                        className="row-button row-button-danger"
                        type="button"
                        onClick={() => {
                          if (window.confirm(`确认删除「${item.name}」？`)) {
                            removeItem(item.id);
                          }
                        }}
                      >
                        删除
                      </button>
                    </div>
                  </div>
                ))}
            </div>
          </section>
        )}
      </main>

      {editorSession && (
        <EditorDrawer
          categories={categories}
          editingId={editorSession.editingId}
          initialDraft={editorSession.initialDraft}
          key={editorSession.editingId ?? "new"}
          onClose={closeEditor}
          onDelete={removeItem}
          onSubmit={handleSubmitDraft}
        />
      )}
    </div>
  );
}

function SortableCostRow({
  item,
  readOnly,
  onEdit,
  onToggle,
}: {
  item: GroupRow;
  readOnly: boolean;
  onEdit: () => void;
  onToggle: () => void;
}) {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: item.id });

  const style = {
    transform: CSS.Translate.toString(transform),
    transition,
  };

  return (
    <article
      className={`cost-row-card${isDragging ? " is-dragging" : ""}`}
      ref={setNodeRef}
      style={style}
    >
      {!readOnly && (
        <button
          aria-label={`拖拽排序 ${item.name}`}
          className="drag-handle"
          type="button"
          {...attributes}
          {...listeners}
        >
          <span />
          <span />
        </button>
      )}

      <div className={"cost-row-main" + (readOnly ? " cost-row-readonly" : "")}>
        <div className="item-name-cell">
          <div className="item-name-line">
            <strong>{item.name}</strong>
            <span className="item-currency-chip">{item.currency.toUpperCase()}</span>
          </div>
          {item.note && <small>{item.note}</small>}
        </div>
        <div className="cost-row-meta">
          <span className="cost-row-caption">原价</span>
          <strong>{formatOriginalPrice(item.amount, item.currency, item.billingMonths)}</strong>
        </div>
        <div className="cost-row-price">
          <span className="cost-row-caption">折合 / 年</span>
          <strong className="number-cell">
            {item.annualCny === null ? "待获取" : cnyCurrency.format(item.annualCny)}
          </strong>
        </div>
        {!readOnly && (
          <div className="row-actions">
            <button className="row-button" type="button" onClick={onEdit}>
              编辑
            </button>
            <button className="row-button" type="button" onClick={onToggle}>
              停用
            </button>
          </div>
        )}
      </div>
    </article>
  );
}

function EditorDrawer({
  categories,
  editingId,
  initialDraft,
  onClose,
  onDelete,
  onSubmit,
}: {
  categories: string[];
  editingId: string | null;
  initialDraft: DraftItem;
  onClose: () => void;
  onDelete: (id: string) => void;
  onSubmit: (draft: DraftItem, editingId: string | null) => void;
}) {
  const [draft, setDraft] = useState(initialDraft);
  const [showCustomCategory, setShowCustomCategory] = useState(() => {
    const initialCategory = initialDraft.category.trim().toLowerCase();
    return (
      initialCategory.length > 0 &&
      !categories.some((category) => category.toLowerCase() === initialCategory)
    );
  });
  const currencyOptions = useMemo(() => {
    const currentCurrency = draft.currency.trim().toUpperCase();
    if (
      !currentCurrency ||
      commonCurrencies.some((currency) => currency.code === currentCurrency)
    ) {
      return commonCurrencies;
    }

    return [
      { code: currentCurrency, label: "当前项目币种" },
      ...commonCurrencies,
    ];
  }, [draft.currency]);
  const isEditing = editingId !== null;
  const categorySelectValue =
    showCustomCategory || !categories.includes(draft.category)
      ? "__placeholder__"
      : draft.category;

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    onSubmit(draft, editingId);
  }

  return (
    <div className="drawer-backdrop" onClick={onClose}>
      <aside
        aria-label={isEditing ? "编辑项目" : "添加项目"}
        className="editor-drawer"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="drawer-header">
          <div>
            <p className="panel-label">{isEditing ? "编辑项目" : "新建项目"}</p>
            <h2>{isEditing ? draft.name || "修改配置" : "添加新的固定支出"}</h2>
          </div>
          <button className="icon-button" type="button" onClick={onClose} aria-label="关闭">
            <svg viewBox="0 0 24 24" width="16" height="16" stroke="currentColor" strokeWidth="2" fill="none" strokeLinecap="round" strokeLinejoin="round">
              <line x1="18" y1="6" x2="6" y2="18"></line>
              <line x1="6" y1="6" x2="18" y2="18"></line>
            </svg>
          </button>
        </div>

        <form className="editor-form" onSubmit={handleSubmit}>
          <div className="editor-form-grid">
            <label className="editor-field">
              <span className="field-label">项目名称</span>
              <input
                autoComplete="off"
                enterKeyHint="next"
                placeholder="例如 iCloud+ / Claude / 域名"
                required
                spellCheck={false}
                type="text"
                value={draft.name}
                onChange={(event) =>
                  setDraft((current) => ({ ...current, name: event.target.value }))
                }
              />
            </label>

            <div className="editor-field">
              <span className="field-label">分类</span>
              <div className="category-picker">
                <div className="category-select-row">
                  <select
                    required={!showCustomCategory}
                    value={categorySelectValue}
                    onChange={(event) => {
                      setShowCustomCategory(false);
                      setDraft((current) => ({
                        ...current,
                        category: event.target.value,
                      }));
                    }}
                  >
                    <option disabled value="__placeholder__">
                      选择分类
                    </option>
                    {categories.map((category) => (
                      <option key={category} value={category}>
                        {category}
                      </option>
                    ))}
                  </select>
                  <button
                    className="secondary-button category-add-button"
                    type="button"
                    onClick={() => {
                      if (showCustomCategory) {
                        setShowCustomCategory(false);
                        setDraft((current) => ({ ...current, category: "" }));
                        return;
                      }

                      setShowCustomCategory(true);
                      setDraft((current) => ({
                        ...current,
                        category:
                          current.category.trim() &&
                          !categories.includes(current.category)
                            ? current.category
                            : "",
                      }));
                    }}
                  >
                    {showCustomCategory ? "取消新增" : "新增分类"}
                  </button>
                </div>

                {showCustomCategory && (
                  <input
                    autoComplete="off"
                    autoFocus
                    enterKeyHint="next"
                    placeholder="输入新分类名称"
                    required
                    spellCheck={false}
                    type="text"
                    value={draft.category}
                    onChange={(event) =>
                      setDraft((current) => ({ ...current, category: event.target.value }))
                    }
                  />
                )}
              </div>
            </div>

            <div className="pricing-grid editor-field-wide">
              <label className="editor-field">
                <span className="field-label">金额</span>
                <input
                  inputMode="decimal"
                  min="0"
                  placeholder="0.00"
                  required
                  step="0.01"
                  type="number"
                  value={draft.amount}
                  onChange={(event) =>
                    setDraft((current) => ({ ...current, amount: event.target.value }))
                  }
                />
              </label>

              <label className="editor-field">
                <span className="field-label">币种</span>
                <select
                  value={draft.currency}
                  onChange={(event) =>
                    setDraft((current) => ({
                      ...current,
                      currency: event.target.value.toUpperCase(),
                    }))
                  }
                >
                  {currencyOptions.map((currency) => (
                    <option key={currency.code} value={currency.code}>
                      {currency.code} · {currency.label}
                    </option>
                  ))}
                </select>
              </label>

              <label className="editor-field">
                <span className="field-label">计费周期</span>
                <select
                  value={draft.billingMonths}
                  onChange={(event) =>
                    setDraft((current) => ({
                      ...current,
                      billingMonths: event.target.value,
                    }))
                  }
                >
                  {billingOptions.map((option) => (
                    <option key={option.months} value={option.months}>
                      {option.label}
                    </option>
                  ))}
                </select>
              </label>
            </div>

            <label className="editor-field editor-field-wide">
              <span className="field-label">备注</span>
              <textarea
                placeholder="可选，例如套餐说明、到期时间、购买渠道"
                rows={3}
                value={draft.note}
                onChange={(event) =>
                  setDraft((current) => ({ ...current, note: event.target.value }))
                }
              />
            </label>

            <label className="checkbox-row editor-field-wide">
              <div className="checkbox-copy">
                <strong>保存后立即纳入统计</strong>
                <span>关闭后会进入已停用项目，但不会删除记录。</span>
              </div>
              <span className="toggle-control">
                <input
                  checked={draft.enabled}
                  type="checkbox"
                  onChange={(event) =>
                    setDraft((current) => ({ ...current, enabled: event.target.checked }))
                  }
                />
                <span className="toggle-track" aria-hidden="true">
                  <span className="toggle-thumb" />
                </span>
              </span>
            </label>
          </div>

          <div className="drawer-actions">
            {isEditing && (
              <button
                className="danger-button"
                type="button"
                onClick={() => {
                  if (editingId && window.confirm(`确认删除「${draft.name}」？`)) {
                    onDelete(editingId);
                    onClose();
                  }
                }}
              >
                删除
              </button>
            )}
            <button className="secondary-button" type="button" onClick={onClose}>
              取消
            </button>
            <button className="primary-button" type="submit">
              {isEditing ? "保存修改" : "添加项目"}
            </button>
          </div>
        </form>
      </aside>
    </div>
  );
}

function annualize(amount: number, billingMonths: BillingMonths) {
  return amount * (12 / billingMonths);
}

function getRateToCny(currency: string, snapshot: FxSnapshot | null) {
  const normalizedCurrency = currency.toUpperCase();

  if (normalizedCurrency === "CNY") {
    return 1;
  }

  if (!snapshot?.rates.CNY) {
    return null;
  }

  if (normalizedCurrency === "EUR") {
    return snapshot.rates.CNY;
  }

  const eurToCurrency = snapshot.rates[normalizedCurrency];
  if (!eurToCurrency) {
    return null;
  }

  return snapshot.rates.CNY / eurToCurrency;
}

function formatOriginalPrice(
  amount: number,
  currency: string,
  billingMonths: BillingMonths,
) {
  return `${trimTrailingZero(amount)} ${currency.toUpperCase()}/${billingLabel(billingMonths)}`;
}

function billingLabel(billingMonths: BillingMonths) {
  switch (billingMonths) {
    case 1:
      return "月";
    case 3:
      return "季";
    case 6:
      return "半年";
    case 12:
      return "年";
    default:
      return "周期";
  }
}

function trimTrailingZero(value: number) {
  return value.toLocaleString("en-US", {
    minimumFractionDigits: 0,
    maximumFractionDigits: 2,
  });
}

function makeId() {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID();
  }

  return `cost-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
}

function hydrateItems(parsed: unknown): CostItem[] {
  if (!Array.isArray(parsed) || parsed.length === 0) {
    return normalizeCategoryOrders(initialItems);
  }
  return normalizeCategoryOrders(
    (parsed as Array<Partial<CostItem>>).map((item, index) => ({
      id: item.id ?? makeId(),
      name: item.name ?? "未命名项目",
      category: item.category ?? "未分类",
      amount: typeof item.amount === "number" ? item.amount : 0,
      currency: item.currency ?? "CNY",
      billingMonths: normalizeBillingMonths(item.billingMonths),
      enabled: item.enabled ?? true,
      note: item.note,
      order: typeof item.order === "number" ? item.order : index,
    })),
  );
}

function loadItems() {
  const raw = window.localStorage.getItem(STORAGE_KEY);
  if (!raw) {
    return normalizeCategoryOrders(initialItems);
  }

  try {
    return hydrateItems(JSON.parse(raw));
  } catch {
    return normalizeCategoryOrders(initialItems);
  }
}

async function fetchItemsFromApi(): Promise<{
  items: CostItem[];
  initialized: boolean;
} | null> {
  const token = getToken();
  try {
    const headers: Record<string, string> = { Accept: "application/json" };
    if (token) {
      headers.Authorization = `Bearer ${token}`;
    }
    const res = await fetch(API_ITEMS_URL, { headers });
    if (res.status === 401) {
      clearToken();
      window.location.reload();
      return null;
    }
    if (!res.ok) return null;
    const data = (await res.json()) as unknown;
    if (!Array.isArray(data)) return null;
    const initialized = res.headers.get("X-Initialized") === "1";
    if (initialized && data.length === 0) {
      return { items: [], initialized: true };
    }
    return { items: hydrateItems(data), initialized };
  } catch {
    return null;
  }
}

async function saveItemsToApi(items: CostItem[]): Promise<boolean> {
  const token = getToken();
  if (!token) return false;
  try {
    const res = await fetch(API_ITEMS_URL, {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify(items),
    });
    if (res.status === 401) {
      clearToken();
      window.location.reload();
      return false;
    }
    return res.ok;
  } catch {
    return false;
  }
}

function loadFxCache() {
  const raw = window.localStorage.getItem(FX_CACHE_KEY);
  if (!raw) return null;

  try {
    const parsed = JSON.parse(raw) as FxSnapshot;
    if (!parsed?.rates || !parsed?.date || !parsed?.fetchedAt) return null;
    if (Date.now() - new Date(parsed.fetchedAt).getTime() > 86_400_000) return null;
    return parsed;
  } catch {
    return null;
  }
}

function formatTimestamp(value?: string) {
  if (!value) {
    return "未记录";
  }

  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "未记录";
  }

  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}

function normalizeBillingMonths(value?: number): BillingMonths {
  if (value === 1 || value === 3 || value === 6 || value === 12) {
    return value;
  }

  return 12;
}

function normalizeCategoryOrders(items: CostItem[]) {
  const byCategory = new Map<string, CostItem[]>();

  for (const item of items) {
    const categoryItems = byCategory.get(item.category) ?? [];
    categoryItems.push(item);
    byCategory.set(item.category, categoryItems);
  }

  const nextOrders = new Map<string, number>();

  for (const categoryItems of byCategory.values()) {
    const activeItems = categoryItems
      .filter((item) => item.enabled)
      .sort((left, right) => left.order - right.order);
    const disabledItems = categoryItems
      .filter((item) => !item.enabled)
      .sort((left, right) => left.order - right.order);

    [...activeItems, ...disabledItems].forEach((item, index) => {
      nextOrders.set(item.id, index);
    });
  }

  let hasChanges = false;
  const normalizedItems = items.map((item) => {
    const nextOrder = nextOrders.get(item.id) ?? item.order;
    if (nextOrder !== item.order) {
      hasChanges = true;
      return { ...item, order: nextOrder };
    }

    return item;
  });

  return hasChanges ? normalizedItems : items;
}

function getNextOrder(items: CostItem[], category: string) {
  const categoryOrders = items
    .filter((item) => item.category === category)
    .map((item) => item.order);

  if (categoryOrders.length === 0) {
    return 0;
  }

  return Math.max(...categoryOrders) + 1;
}

export default App;
