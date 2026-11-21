// Reads a feed item (a news story or a fresh mentor post) inside Radar, in a
// side sheet, instead of sending the learner to another site.
import {
  Button,
  Sheet,
  SheetContent,
  SheetDescription,
  SheetTitle,
} from '@hub/ui-kit';
import { useQueryClient } from '@tanstack/react-query';
import { Bookmark, BookmarkCheck, Check, ThumbsUp } from 'lucide-react';
import {
  createContext,
  use,
  useCallback,
  useState,
  type ReactNode,
} from 'react';
import { api, type Item } from './api';
import { ago } from './format';
import { keys, useDone } from './queries';
import { Face } from './look';
import { OpenOutside, Viewer } from './viewer';

const OpenItem = createContext<(it: Item) => void>(() => undefined);

/** useOpenItem returns a function that opens an item in the reader sheet. */
export const useOpenItem = () => use(OpenItem);

export function ItemSheetProvider({
  children,
  onRead,
}: {
  children: ReactNode;
  onRead?: () => void;
}) {
  const qc = useQueryClient();
  const [item, setItem] = useState<Item | null>(null);
  const [open, setOpen] = useState(false);
  const show = useCallback(
    (it: Item) => {
      setItem(it);
      setOpen(true);
      if (!it.read && !it.source.mentor)
        void api.setRead(it.id, true).then(() => {
          void qc.invalidateQueries({ queryKey: keys.newsPanel });
          onRead?.();
        });
    },
    [qc, onRead],
  );
  return (
    <OpenItem value={show}>
      {children}
      <Sheet open={open} onOpenChange={setOpen}>
        <SheetContent
          className="@container overflow-y-auto"
          onOpenAutoFocus={(e) => e.preventDefault()}
        >
          {item && <ItemBody item={item} onClose={() => setOpen(false)} />}
        </SheetContent>
      </Sheet>
    </OpenItem>
  );
}

function ItemBody({ item, onClose }: { item: Item; onClose: () => void }) {
  const qc = useQueryClient();
  const done = useDone();
  const [saved, setSaved] = useState(item.saved);
  const mentor = !!item.source.mentor;
  return (
    <div className="flex min-h-full flex-col">
      <div className="flex gap-3 border-b px-5 pt-5 pb-4 pr-12">
        {mentor && <Face id={item.source.mentor} size="md" />}
        <div className="min-w-0 space-y-1">
          <SheetDescription>
            {item.source.name} · {ago(item.publishedAt)}
          </SheetDescription>
          <SheetTitle className="text-lg leading-snug">{item.title}</SheetTitle>
        </div>
      </div>
      <div className="flex-1 px-5 py-5">
        <Viewer refId={`item:${item.id}`} compact />
      </div>
      <div className="sticky bottom-0 flex flex-wrap items-center gap-2 border-t bg-background/90 px-5 py-3 backdrop-blur">
        <OpenOutside url={item.url} label="Original" variant="ghost" />
        <div className="flex-1" />
        {!mentor && (
          <Button
            variant="ghost"
            size="sm"
            onClick={() => {
              setSaved(!saved);
              void api
                .setSaved(item.id, !saved)
                .then(() => qc.invalidateQueries({ queryKey: keys.newsPanel }));
            }}
          >
            {saved ? <BookmarkCheck className="text-primary" /> : <Bookmark />}
            {saved ? 'Saved' : 'Save'}
          </Button>
        )}
        {mentor && (
          <>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                done.mutate({
                  ref: `item:${item.id}`,
                  rating: '',
                  worth: true,
                });
                void qc.invalidateQueries({ queryKey: ['fresh'] });
                onClose();
              }}
            >
              <ThumbsUp /> Worth it
            </Button>
            <Button
              size="sm"
              onClick={() => {
                done.mutate({
                  ref: `item:${item.id}`,
                  rating: '',
                  worth: null,
                });
                void qc.invalidateQueries({ queryKey: ['fresh'] });
                onClose();
              }}
            >
              <Check /> Done
            </Button>
          </>
        )}
      </div>
    </div>
  );
}
