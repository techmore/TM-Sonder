import Foundation
import SwiftUI
import UniformTypeIdentifiers

nonisolated struct SonderBookListEntry: Identifiable, Hashable {
    var id: String { "\(order)-\(title)" }
    let order: Int
    let title: String
    let author: String
}

nonisolated struct SonderBookList: Identifiable, Hashable {
    let id: String
    let name: String
    let subtitle: String
    let icon: String
    let entries: [SonderBookListEntry]

    /// A broad, evergreen catalog of canonical and highly recommended shelves.
    /// These are intentionally starter lists rather than claims of one definitive ranking.
    static let curated: [SonderBookList] = [
        make("all-time-greats", "Best Books of All Time", "A strong first pass through universally acclaimed reading.", ["Middlemarch — George Eliot", "The Great Gatsby — F. Scott Fitzgerald", "One Hundred Years of Solitude — Gabriel García Márquez", "Beloved — Toni Morrison", "Ulysses — James Joyce"]),
        make("all-time-novels", "Best Novels of All Time", "A canon of the most acclaimed novels across centuries.", ["Don Quixote — Miguel de Cervantes", "War and Peace — Leo Tolstoy", "Madame Bovary — Gustave Flaubert", "The Great Gatsby — F. Scott Fitzgerald", "Beloved — Toni Morrison"]),
        make("all-time-nonfiction", "Best Nonfiction of All Time", "Landmark nonfiction with lasting influence.", ["The Republic — Plato", "The Histories — Herodotus", "On the Origin of Species — Charles Darwin", "The Diary of a Young Girl — Anne Frank", "Silent Spring — Rachel Carson"]),
        make("all-time-american", "Best American Books of All Time", "A definitive starting shelf for American literature.", ["Moby-Dick — Herman Melville", "The Adventures of Huckleberry Finn — Mark Twain", "The Great Gatsby — F. Scott Fitzgerald", "Invisible Man — Ralph Ellison", "Beloved — Toni Morrison"]),
        make("all-time-british", "Best British Books of All Time", "Essential British novels, plays, and poetry.", ["Pride and Prejudice — Jane Austen", "Jane Eyre — Charlotte Brontë", "Middlemarch — George Eliot", "Mrs Dalloway — Virginia Woolf", "Hamlet — William Shakespeare"]),
        make("all-time-world", "Best World Literature of All Time", "The most enduring international classics.", ["The Iliad — Homer", "The Divine Comedy — Dante Alighieri", "The Tale of Genji — Murasaki Shikibu", "The Brothers Karamazov — Fyodor Dostoevsky", "One Hundred Years of Solitude — Gabriel García Márquez"]),
        make("all-time-20th-century", "Best Books of the 20th Century", "The defining books of the modern century.", ["In Search of Lost Time — Marcel Proust", "The Sound and the Fury — William Faulkner", "The Magic Mountain — Thomas Mann", "1984 — George Orwell", "The Name of the Rose — Umberto Eco"]),
        make("all-time-21st-century", "Best Books of the 21st Century", "A high-signal selection from contemporary literature.", ["The Corrections — Jonathan Franzen", "The Road — Cormac McCarthy", "2666 — Roberto Bolaño", "The Brief Wondrous Life of Oscar Wao — Junot Díaz", "The Overstory — Richard Powers"]),
        make("all-time-short-books", "Best Short Books of All Time", "Canonical books with enormous ideas in small packages.", ["The Little Prince — Antoine de Saint-Exupéry", "The Death of Ivan Ilyich — Leo Tolstoy", "The Stranger — Albert Camus", "The Metamorphosis — Franz Kafka", "The Old Man and the Sea — Ernest Hemingway"]),
        make("all-time-long-books", "Best Long Books of All Time", "Big, immersive novels worth the commitment.", ["The Count of Monte Cristo — Alexandre Dumas", "Les Misérables — Victor Hugo", "War and Peace — Leo Tolstoy", "The Lord of the Rings — J. R. R. Tolkien", "Infinite Jest — David Foster Wallace"]),
        make("all-time-debut-novels", "Best Debut Novels of All Time", "First novels that announced major literary voices.", ["Frankenstein — Mary Shelley", "The Bell Jar — Sylvia Plath", "The God of Small Things — Arundhati Roy", "The Secret History — Donna Tartt", "The Kite Runner — Khaled Hosseini"]),
        make("all-time-scifi", "Best Science Fiction of All Time", "The essential science-fiction canon.", ["The War of the Worlds — H. G. Wells", "Brave New World — Aldous Huxley", "Dune — Frank Herbert", "The Left Hand of Darkness — Ursula K. Le Guin", "Snow Crash — Neal Stephenson"]),
        make("all-time-fantasy", "Best Fantasy of All Time", "The fantasy books that shaped the genre.", ["The Hobbit — J. R. R. Tolkien", "The Lord of the Rings — J. R. R. Tolkien", "A Wizard of Earthsea — Ursula K. Le Guin", "The Last Unicorn — Peter S. Beagle", "A Game of Thrones — George R. R. Martin"]),
        make("all-time-mystery", "Best Mystery Novels of All Time", "Unforgettable puzzles, detectives, and crimes.", ["The Murder of Roger Ackroyd — Agatha Christie", "The Hound of the Baskervilles — Arthur Conan Doyle", "The Big Sleep — Raymond Chandler", "The Name of the Rose — Umberto Eco", "The Girl with the Dragon Tattoo — Stieg Larsson"]),
        make("all-time-horror", "Best Horror Books of All Time", "The essential canon of literary and popular horror.", ["Dracula — Bram Stoker", "Frankenstein — Mary Shelley", "The Haunting of Hill House — Shirley Jackson", "The Exorcist — William Peter Blatty", "The Shining — Stephen King"]),
        make("all-time-romance", "Best Romance Novels of All Time", "Enduring stories about love, longing, and partnership.", ["Pride and Prejudice — Jane Austen", "Jane Eyre — Charlotte Brontë", "Wuthering Heights — Emily Brontë", "Persuasion — Jane Austen", "Love in the Time of Cholera — Gabriel García Márquez"]),
        make("all-time-biographies", "Best Biographies of All Time", "Lives that illuminate character, power, and history.", ["The Life of Samuel Johnson — James Boswell", "The Power Broker — Robert A. Caro", "Long Walk to Freedom — Nelson Mandela", "The Autobiography of Malcolm X — Malcolm X", "Steve Jobs — Walter Isaacson"]),
        make("all-time-memoirs", "Best Memoirs of All Time", "Personal testimony with lasting literary power.", ["The Confessions — Augustine of Hippo", "The Autobiography of Benjamin Franklin — Benjamin Franklin", "Night — Elie Wiesel", "The Year of Magical Thinking — Joan Didion", "Educated — Tara Westover"]),
        make("all-time-philosophy", "Best Philosophy Books of All Time", "Foundational works for thinking clearly about life.", ["Meditations — Marcus Aurelius", "The Republic — Plato", "Nicomachean Ethics — Aristotle", "The Prince — Niccolò Machiavelli", "Being and Time — Martin Heidegger"]),
        make("all-time-poetry", "Best Poetry Books of All Time", "Poetry collections and epics that changed the form.", ["The Iliad — Homer", "The Divine Comedy — Dante Alighieri", "Leaves of Grass — Walt Whitman", "The Waste Land — T. S. Eliot", "The Complete Poems — Emily Dickinson"]),
        make("all-time-childrens", "Best Children's Books of All Time", "The childhood classics that never stop working.", ["Alice's Adventures in Wonderland — Lewis Carroll", "The Wind in the Willows — Kenneth Grahame", "Charlotte's Web — E. B. White", "The Chronicles of Narnia — C. S. Lewis", "The Phantom Tollbooth — Norton Juster"]),
        make("modern-classics", "Modern Classics", "Widely loved fiction that belongs on a lifetime shelf.", ["The Great Gatsby — F. Scott Fitzgerald", "To Kill a Mockingbird — Harper Lee", "1984 — George Orwell", "Brave New World — Aldous Huxley", "The Catcher in the Rye — J. D. Salinger"]),
        make("literary-fiction", "Literary Fiction Essentials", "A high-signal shelf of ambitious, beautifully written novels.", ["Anna Karenina — Leo Tolstoy", "The Brothers Karamazov — Fyodor Dostoevsky", "The Remains of the Day — Kazuo Ishiguro", "The Goldfinch — Donna Tartt", "Atonement — Ian McEwan"]),
        make("novels-everyone-should-read", "Novels Everyone Should Read", "Conversation-making fiction from many eras and countries.", ["Pride and Prejudice — Jane Austen", "The Count of Monte Cristo — Alexandre Dumas", "The Kite Runner — Khaled Hosseini", "The Book Thief — Markus Zusak", "The Handmaid's Tale — Margaret Atwood"]),
        make("short-classics", "Short Classics", "Great books you can finish in a weekend.", ["Of Mice and Men — John Steinbeck", "The Metamorphosis — Franz Kafka", "Animal Farm — George Orwell", "The Stranger — Albert Camus", "The Old Man and the Sea — Ernest Hemingway"]),
        make("american-literature", "American Literature Canon", "Foundational voices in American fiction.", ["Moby-Dick — Herman Melville", "The Adventures of Huckleberry Finn — Mark Twain", "Their Eyes Were Watching God — Zora Neale Hurston", "Invisible Man — Ralph Ellison", "The Grapes of Wrath — John Steinbeck"]),
        make("world-literature", "World Literature Essentials", "Landmark novels beyond the English-language canon.", ["The Master and Margarita — Mikhail Bulgakov", "Things Fall Apart — Chinua Achebe", "The Trial — Franz Kafka", "The Unbearable Lightness of Being — Milan Kundera", "The Savage Detectives — Roberto Bolaño"]),
        make("women-writers", "Essential Women Writers", "Powerful, influential books by women writers.", ["Jane Eyre — Charlotte Brontë", "The Awakening — Kate Chopin", "Mrs Dalloway — Virginia Woolf", "The Color Purple — Alice Walker", "The Poisonwood Bible — Barbara Kingsolver"]),
        make("translated-fiction", "Great Translated Fiction", "International favorites in English translation.", ["The Name of the Rose — Umberto Eco", "The Shadow of the Wind — Carlos Ruiz Zafón", "The Door — Magda Szabó", "The Memory Police — Yoko Ogawa", "The Years — Annie Ernaux"]),
        make("pulitzer-fiction", "Pulitzer Prize Fiction", "A starter shelf of Pulitzer-recognized American novels.", ["The Road — Cormac McCarthy", "The Underground Railroad — Colson Whitehead", "The Nickel Boys — Colson Whitehead", "The Known World — Edward P. Jones", "Olive Kitteridge — Elizabeth Strout"]),
        make("booker-winners", "Booker Prize Winners", "Modern English-language fiction recognized by the Booker.", ["The English Patient — Michael Ondaatje", "Wolf Hall — Hilary Mantel", "The Remains of the Day — Kazuo Ishiguro", "The Sea — John Banville", "Shuggie Bain — Douglas Stuart"]),
        make("nobel-literature", "Nobel Laureates in Literature", "Accessible starting points from Nobel-recognized authors.", ["The Plague — Albert Camus", "The Old Man and the Sea — Ernest Hemingway", "Blindness — José Saramago", "The Death of Artemio Cruz — Carlos Fuentes", "The Grapes of Wrath — John Steinbeck"]),
        make("fantasy-essentials", "Fantasy Essentials", "The most influential modern fantasy journeys.", ["The Hobbit — J. R. R. Tolkien", "The Fellowship of the Ring — J. R. R. Tolkien", "A Wizard of Earthsea — Ursula K. Le Guin", "The Name of the Wind — Patrick Rothfuss", "The Fifth Season — N. K. Jemisin"]),
        make("epic-fantasy-series", "Epic Fantasy Series", "Big worlds, long arcs, and fully immersive series starters.", ["A Game of Thrones — George R. R. Martin", "The Way of Kings — Brandon Sanderson", "The Eye of the World — Robert Jordan", "The Blade Itself — Joe Abercrombie", "Gardens of the Moon — Steven Erikson"]),
        make("science-fiction-essentials", "Science Fiction Essentials", "Books that define the possibilities of science fiction.", ["Dune — Frank Herbert", "Foundation — Isaac Asimov", "The Left Hand of Darkness — Ursula K. Le Guin", "Neuromancer — William Gibson", "The Dispossessed — Ursula K. Le Guin"]),
        make("recent-scifi", "Modern Science Fiction", "Recent, highly recommended speculative fiction.", ["Project Hail Mary — Andy Weir", "Children of Time — Adrian Tchaikovsky", "Annihilation — Jeff VanderMeer", "The Three-Body Problem — Cixin Liu", "Station Eleven — Emily St. John Mandel"]),
        make("dystopian-fiction", "Dystopian Fiction", "Classic warnings about power, conformity, and technology.", ["1984 — George Orwell", "Brave New World — Aldous Huxley", "Fahrenheit 451 — Ray Bradbury", "The Handmaid's Tale — Margaret Atwood", "Never Let Me Go — Kazuo Ishiguro"]),
        make("horror-classics", "Horror Classics", "Foundational novels of dread, gothic atmosphere, and the uncanny.", ["Dracula — Bram Stoker", "Frankenstein — Mary Shelley", "The Haunting of Hill House — Shirley Jackson", "The Shining — Stephen King", "The Exorcist — William Peter Blatty"]),
        make("mystery-detective", "Mystery & Detective Greats", "Essential puzzles and investigations.", ["The Hound of the Baskervilles — Arthur Conan Doyle", "The Murder of Roger Ackroyd — Agatha Christie", "The Big Sleep — Raymond Chandler", "In Cold Blood — Truman Capote", "The Girl with the Dragon Tattoo — Stieg Larsson"]),
        make("thrillers", "Page-Turning Thrillers", "Popular thrillers that are hard to put down.", ["Gone Girl — Gillian Flynn", "The Silence of the Lambs — Thomas Harris", "The Da Vinci Code — Dan Brown", "The Talented Mr. Ripley — Patricia Highsmith", "The Day of the Jackal — Frederick Forsyth"]),
        make("historical-fiction", "Historical Fiction Favorites", "Fiction that makes history vivid.", ["All the Light We Cannot See — Anthony Doerr", "The Pillars of the Earth — Ken Follett", "The Book Thief — Markus Zusak", "Wolf Hall — Hilary Mantel", "The Nightingale — Kristin Hannah"]),
        make("romance-classics", "Romance Classics", "Enduring love stories and relationship novels.", ["Pride and Prejudice — Jane Austen", "Jane Eyre — Charlotte Brontë", "Persuasion — Jane Austen", "Wuthering Heights — Emily Brontë", "Love in the Time of Cholera — Gabriel García Márquez"]),
        make("poetry-essential", "Poetry Essentials", "A welcoming path through essential poets and collections.", ["The Complete Poems — Emily Dickinson", "Leaves of Grass — Walt Whitman", "The Waste Land — T. S. Eliot", "Ariel — Sylvia Plath", "The Essential Rumi — Rumi"]),
        make("plays", "Great Plays", "Plays that remain central to world literature.", ["Hamlet — William Shakespeare", "The Tempest — William Shakespeare", "A Doll's House — Henrik Ibsen", "Waiting for Godot — Samuel Beckett", "Death of a Salesman — Arthur Miller"]),
        make("mythology", "Mythology & Epics", "The stories beneath much of Western literature.", ["The Iliad — Homer", "The Odyssey — Homer", "The Aeneid — Virgil", "Metamorphoses — Ovid", "The Epic of Gilgamesh — Anonymous"]),
        make("ancient-philosophy", "Ancient Philosophy", "Foundational works for thinking about virtue and society.", ["The Republic — Plato", "Meditations — Marcus Aurelius", "Nicomachean Ethics — Aristotle", "Letters from a Stoic — Seneca", "The Art of War — Sun Tzu"]),
        make("philosophy", "Philosophy for Curious Readers", "A readable route through major philosophical questions.", ["Beyond Good and Evil — Friedrich Nietzsche", "Man's Search for Meaning — Viktor E. Frankl", "The Second Sex — Simone de Beauvoir", "The Myth of Sisyphus — Albert Camus", "The Ethics of Ambiguity — Simone de Beauvoir"]),
        make("psychology", "Psychology & Human Nature", "Books for understanding minds, behavior, and bias.", ["Thinking, Fast and Slow — Daniel Kahneman", "Influence — Robert Cialdini", "The Righteous Mind — Jonathan Haidt", "The Man Who Mistook His Wife for a Hat — Oliver Sacks", "Behave — Robert Sapolsky"]),
        make("popular-science", "Popular Science", "Big ideas in science explained for general readers.", ["A Brief History of Time — Stephen Hawking", "Cosmos — Carl Sagan", "The Selfish Gene — Richard Dawkins", "The Gene — Siddhartha Mukherjee", "The Emperor of All Maladies — Siddhartha Mukherjee"]),
        make("evolution", "Evolution & Biology", "A readable foundation in life and evolution.", ["On the Origin of Species — Charles Darwin", "The Blind Watchmaker — Richard Dawkins", "Your Inner Fish — Neil Shubin", "The Vital Question — Nick Lane", "The Song of the Dodo — David Quammen"]),
        make("history", "History Everyone Should Know", "High-impact books for building historical context.", ["Guns, Germs, and Steel — Jared Diamond", "A People's History of the United States — Howard Zinn", "The Silk Roads — Peter Frankopan", "The Wright Brothers — David McCullough", "The Warmth of Other Suns — Isabel Wilkerson"]),
        make("ancient-history", "Ancient World History", "Civilizations, empires, and ideas that shaped the world.", ["SPQR — Mary Beard", "The Histories — Herodotus", "The Rise and Fall of Ancient Egypt — Toby Wilkinson", "Rubicon — Tom Holland", "The Persian Fire — Tom Holland"]),
        make("world-wars", "World War History", "Essential accounts of the world wars and their aftermath.", ["The Rise and Fall of the Third Reich — William L. Shirer", "The Guns of August — Barbara W. Tuchman", "With the Old Breed — E. B. Sledge", "Band of Brothers — Stephen E. Ambrose", "Postwar — Tony Judt"]),
        make("biography", "Great Biographies", "Lives that illuminate ambition, creativity, and history.", ["Steve Jobs — Walter Isaacson", "Leonardo da Vinci — Walter Isaacson", "Alexander Hamilton — Ron Chernow", "The Power Broker — Robert A. Caro", "Catherine the Great — Robert K. Massie"]),
        make("memoir", "Memoirs Worth Reading", "Personal stories with lasting cultural and emotional impact.", ["Educated — Tara Westover", "The Year of Magical Thinking — Joan Didion", "When Breath Becomes Air — Paul Kalanithi", "Night — Elie Wiesel", "The Glass Castle — Jeannette Walls"]),
        make("essays", "Great Essay Collections", "Sharp, influential nonfiction in essay form.", ["On Writing — George Orwell", "The White Album — Joan Didion", "Consider the Lobster — David Foster Wallace", "The Fire Next Time — James Baldwin", "A Room of One's Own — Virginia Woolf"]),
        make("self-improvement", "Best Self-Improvement Books", "Practical books for habits, focus, and a better life.", ["Atomic Habits — James Clear", "The 7 Habits of Highly Effective People — Stephen R. Covey", "How to Win Friends and Influence People — Dale Carnegie", "Deep Work — Cal Newport", "Four Thousand Weeks — Oliver Burkeman"]),
        make("business", "Business Classics", "Enduring books about organizations, markets, and strategy.", ["Good to Great — Jim Collins", "The Innovator's Dilemma — Clayton M. Christensen", "The Lean Startup — Eric Ries", "Built to Last — Jim Collins", "The Hard Thing About Hard Things — Ben Horowitz"]),
        make("leadership", "Leadership & Management", "Practical and humane approaches to leading people.", ["The Effective Executive — Peter F. Drucker", "Turn the Ship Around! — L. David Marquet", "Dare to Lead — Brené Brown", "Radical Candor — Kim Scott", "High Output Management — Andrew S. Grove"]),
        make("economics", "Economics for Everyone", "Accessible books for understanding incentives and society.", ["The Wealth of Nations — Adam Smith", "Capital in the Twenty-First Century — Thomas Piketty", "Freakonomics — Steven D. Levitt", "Poor Economics — Abhijit V. Banerjee", "The Undercover Economist — Tim Harford"]),
        make("creativity", "Creativity & Craft", "Books for making, writing, and doing better creative work.", ["The Artist's Way — Julia Cameron", "Steal Like an Artist — Austin Kleon", "Bird by Bird — Anne Lamott", "The War of Art — Steven Pressfield", "On Writing — Stephen King"]),
        make("food", "Food & Cooking Books", "Essential food writing, culture, and technique.", ["Salt, Fat, Acid, Heat — Samin Nosrat", "The Food Lab — J. Kenji López-Alt", "Kitchen Confidential — Anthony Bourdain", "The Omnivore's Dilemma — Michael Pollan", "The Art of Fermentation — Sandor Ellix Katz"]),
        make("nature", "Nature Writing", "Books that deepen attention to the living world.", ["Walden — Henry David Thoreau", "Braiding Sweetgrass — Robin Wall Kimmerer", "The Hidden Life of Trees — Peter Wohlleben", "H is for Hawk — Helen Macdonald", "Pilgrim at Tinker Creek — Annie Dillard"]),
        make("travel", "Travel & Exploration", "Classic journeys and thoughtful travel writing.", ["In Patagonia — Bruce Chatwin", "A Time of Gifts — Patrick Leigh Fermor", "The Great Railway Bazaar — Paul Theroux", "The Art of Travel — Alain de Botton", "Blue Highways — William Least Heat-Moon"]),
        make("children", "Children's Classics", "Books that reward rereading at every age.", ["Charlotte's Web — E. B. White", "The Secret Garden — Frances Hodgson Burnett", "A Wrinkle in Time — Madeleine L'Engle", "The Wind in the Willows — Kenneth Grahame", "The Phantom Tollbooth — Norton Juster"]),
        make("young-adult", "Young Adult Essentials", "Beloved coming-of-age stories for teen and adult readers.", ["The Outsiders — S. E. Hinton", "The Perks of Being a Wallflower — Stephen Chbosky", "The Hate U Give — Angie Thomas", "The Book Thief — Markus Zusak", "His Dark Materials — Philip Pullman"]),
        make("graphic-novels", "Graphic Novels", "Landmark stories told through words and art.", ["Maus — Art Spiegelman", "Watchmen — Alan Moore", "Persepolis — Marjane Satrapi", "The Sandman — Neil Gaiman", "Fun Home — Alison Bechdel"]),
        make("religion-spirituality", "Religion & Spirituality", "Major spiritual texts and modern reflections.", ["The Bible — Various Authors", "The Quran — Various Authors", "The Bhagavad Gita — Anonymous", "The Book of Joy — Dalai Lama", "When Things Fall Apart — Pema Chödrön"]),
        make("social-justice", "Social Justice & Civil Rights", "Essential voices on freedom, race, and equality.", ["The Autobiography of Malcolm X — Malcolm X", "Between the World and Me — Ta-Nehisi Coates", "Just Mercy — Bryan Stevenson", "The New Jim Crow — Michelle Alexander", "Freedom Is a Constant Struggle — Angela Y. Davis"]),
        make("climate", "Climate & Environment", "Books for understanding the planetary challenge.", ["Silent Spring — Rachel Carson", "The Uninhabitable Earth — David Wallace-Wells", "Drawdown — Paul Hawken", "The Sixth Extinction — Elizabeth Kolbert", "Under a White Sky — Elizabeth Kolbert"]),
        make("reading-order-series", "Must-Read Series Starters", "A practical queue for committing to great long series.", ["The Fellowship of the Ring — J. R. R. Tolkien", "A Game of Thrones — George R. R. Martin", "The Eye of the World — Robert Jordan", "The Golden Compass — Philip Pullman", "The Lightning Thief — Rick Riordan"]),
        make("one-book-month", "One Great Book a Month", "A balanced year of high-value reading.", ["January — Pride and Prejudice — Jane Austen", "February — The Alchemist — Paulo Coelho", "March — Sapiens — Yuval Noah Harari", "April — Dune — Frank Herbert", "May — East of Eden — John Steinbeck"]),
        make("book-club", "Book Club Favorites", "Reliable conversation starters for a shared shelf.", ["The Midnight Library — Matt Haig", "Lessons in Chemistry — Bonnie Garmus", "Demon Copperhead — Barbara Kingsolver", "The Seven Husbands of Evelyn Hugo — Taylor Jenkins Reid", "Remarkably Bright Creatures — Shelby Van Pelt"]),
        make("comfort-reads", "Comfort Reads", "Warm, generous, and deeply re-readable books.", ["A Man Called Ove — Fredrik Backman", "The House in the Cerulean Sea — T. J. Klune", "Anne of Green Gables — L. M. Montgomery", "The Guernsey Literary and Potato Peel Pie Society — Mary Ann Shaffer", "The Little Prince — Antoine de Saint-Exupéry"]),
        make("books-that-change-you", "Books That Change You", "A deliberately subjective shelf of perspective-shifting books.", ["Man's Search for Meaning — Viktor E. Frankl", "The Alchemist — Paulo Coelho", "The Four Agreements — Don Miguel Ruiz", "The Death of Ivan Ilyich — Leo Tolstoy", "The Prophet — Kahlil Gibran"]),
        make("american-presidents", "Presidential & Political History", "Books for understanding American power and politics.", ["Team of Rivals — Doris Kearns Goodwin", "The Federalist Papers — Alexander Hamilton", "Fear — Bob Woodward", "The Years of Lyndon Johnson — Robert A. Caro", "The Audacity of Hope — Barack Obama"]),
        make("lifetime-reading", "Lifetime Reading Plan", "A compact personal canon to revisit over decades.", ["The Odyssey — Homer", "Don Quixote — Miguel de Cervantes", "War and Peace — Leo Tolstoy", "The Magic Mountain — Thomas Mann", "The Lord of the Rings — J. R. R. Tolkien"])
    ]

    private static func make(_ id: String, _ name: String, _ subtitle: String, _ books: [String]) -> SonderBookList {
        SonderBookList(id: id, name: name, subtitle: subtitle, icon: "books.vertical", entries: books.enumerated().map { index, value in
            let parts = value.components(separatedBy: " — ")
            let title = parts.count > 2 ? parts.dropLast().joined(separator: " — ") : (parts.first ?? value)
            let author = parts.count > 1 ? (parts.last ?? "") : ""
            return SonderBookListEntry(order: index + 1, title: title, author: author)
        })
    }
}

struct SonderPlainTextDocument: FileDocument {
    static var readableContentTypes: [UTType] { [.plainText] }
    static var writableContentTypes: [UTType] { [.plainText] }
    var text: String

    init(text: String = "") { self.text = text }

    init(configuration: ReadConfiguration) throws {
        text = String(data: configuration.file.regularFileContents ?? Data(), encoding: .utf8) ?? ""
    }

    func fileWrapper(configuration: WriteConfiguration) throws -> FileWrapper {
        FileWrapper(regularFileWithContents: Data(text.utf8))
    }
}

extension SonderLibrary {
    func resolvedBookList(_ list: SonderBookList) -> (matched: [SonderMediaItem], missing: [SonderBookListEntry]) {
        let books = items.filter { $0.kind == .ebook || $0.kind == .audiobook }
        var used = Set<UUID>()
        var matched: [SonderMediaItem] = []
        var missing: [SonderBookListEntry] = []
        for entry in list.entries {
            let key = Self.bookListKey(entry.title)
            if let item = books.first(where: { Self.bookListKey($0.title) == key && used.contains($0.id) == false }) {
                matched.append(item)
                used.insert(item.id)
            } else {
                missing.append(entry)
            }
        }
        return (matched, missing)
    }

    func missingBookListText(_ list: SonderBookList) -> String {
        let missing = resolvedBookList(list).missing
        guard missing.isEmpty == false else { return "No missing books for \(list.name).\n" }
        return (["\(list.name) — missing books", ""] + missing.map { "\($0.order). \($0.title) — \($0.author)" } + [""]).joined(separator: "\n")
    }

    nonisolated private static func bookListKey(_ title: String) -> String {
        title.folding(options: [.diacriticInsensitive, .caseInsensitive], locale: .current)
            .components(separatedBy: CharacterSet.alphanumerics.inverted).joined()
    }
}
