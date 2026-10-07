// @ts-ignore: CSS module import for side effects
import "./ui.css";

type Props = { title: string; text?: string; action?: React.ReactNode };

export default function EmptyState({ title, text, action }: Props) {
  return (
    <div className="ui-empty">
      <h2 className="ui-empty__title">{title}</h2>
      {text ? <p className="ui-empty__text">{text}</p> : null}
      {action}
    </div>
  );
}
