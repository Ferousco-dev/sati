import type { Announcement } from "@/lib/content/types";
// @ts-ignore: CSS module import for side effects
import "@/components/styles/floating.css";

export default function AnnouncementBar({ content }: { content: Announcement }) {
  if (!content.enabled || !content.text) return null;
  return (
    <div className="announce" role="note">
      {content.text}
      {content.link_url && content.link_label ? <a href={content.link_url}>{content.link_label}</a> : null}
    </div>
  );
}
