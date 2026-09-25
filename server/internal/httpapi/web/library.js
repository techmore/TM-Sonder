    let items = [];
    let lists = [];
    let selectedListID = null;
    // Curated-shelf candidate pools, keyed by sorted media kinds. Rebuilt when
    // the catalog or the copy grouping changes.
    const listCandidateState = new Map();
    let listIndexGeneration = 0;
    const progressByID = new Map();

    const TOP_100_BOOKS = [
      "Don Quixote", "Middlemarch", "War and Peace", "The Great Gatsby", "Beloved", "Ulysses", "One Hundred Years of Solitude", "The Brothers Karamazov", "Anna Karenina", "Madame Bovary",
      "Pride and Prejudice", "Jane Eyre", "Moby-Dick", "The Odyssey", "The Iliad", "The Divine Comedy", "Crime and Punishment", "The Count of Monte Cristo", "The Lord of the Rings", "The Hobbit",
      "1984", "Brave New World", "The Catcher in the Rye", "To Kill a Mockingbird", "The Grapes of Wrath", "Invisible Man", "The Sound and the Fury", "The Sun Also Rises", "Their Eyes Were Watching God", "The Color Purple",
      "The Road", "The Handmaid's Tale", "One Flew Over the Cuckoo's Nest", "The Bell Jar", "The Stranger", "The Metamorphosis", "The Trial", "The Master and Margarita", "The Name of the Rose", "The Shadow of the Wind",
      "The Old Man and the Sea", "Of Mice and Men", "The Little Prince", "The Death of Ivan Ilyich", "The Alchemist", "The Book Thief", "The Kite Runner", "The Remains of the Day", "Atonement", "The Goldfinch",
      "The Secret History", "The God of Small Things", "The Brief Wondrous Life of Oscar Wao", "The Underground Railroad", "The Nickel Boys", "The Overstory", "Cloud Atlas", "The Amazing Adventures of Kavalier & Clay", "The Corrections", "2666",
      "Dune", "Foundation", "The Left Hand of Darkness", "Neuromancer", "The Dispossessed", "The War of the Worlds", "Fahrenheit 451", "Snow Crash", "The Three-Body Problem", "Project Hail Mary",
      "A Wizard of Earthsea", "A Game of Thrones", "The Name of the Wind", "The Fifth Season", "The Last Unicorn", "The Once and Future King", "The Chronicles of Narnia", "His Dark Materials", "The Wheel of Time", "The Way of Kings",
      "Dracula", "Frankenstein", "The Haunting of Hill House", "The Shining", "The Exorcist", "The Murder of Roger Ackroyd", "The Hound of the Baskervilles", "The Big Sleep", "Gone Girl", "The Silence of the Lambs",
      "The Republic", "Meditations", "Nicomachean Ethics", "The Prince", "The Art of War", "The Histories", "Sapiens", "A Brief History of Time", "Cosmos", "The Selfish Gene",
      "On the Origin of Species", "Silent Spring", "The Diary of a Young Girl", "Night", "The Autobiography of Malcolm X", "Long Walk to Freedom", "The Power Broker", "Steve Jobs", "Educated", "The Year of Magical Thinking"
    ].slice(0, 100);

    const SOURCE_RECOMMENDATIONS = [
      ["The Guardian · 100 Best Novels of All Time (2026)", "Author, critic, and academic poll with Middlemarch at the top."],
      ["The Guardian · 100 Best Novels in English (2015)", "Robert McCrum's chronological English-language canon."],
      ["The New York Times · 100 Best Books of the 21st Century (2024)", "Critic and author-voted contemporary reading."],
      ["The New York Times · Readers' 100 Best Books of the 21st Century", "The public-vote companion to the NYT century list."],
      ["TIME · All-TIME 100 Novels", "TIME critics' English-language novel canon."],
      ["Modern Library · 100 Best Novels", "The Modern Library editorial board's twentieth-century canon."],
      ["Modern Library · Readers' 100 Best Novels", "The reader-voted companion to the Modern Library list."],
      ["Le Monde / Fnac · 100 Books of the Century", "A French poll of memorable twentieth-century books."],
      ["Bokklubben · World Library", "A global canon of authors from 54 countries."],
      ["The Greatest Books · Aggregated Top 100", "A consensus ranking built from many major best-of lists."],
      ["OCLC WorldCat · The Library 100", "Library-holdings-based cultural staying power."],
      ["PBS · The Great American Read", "America's 100 most-loved books, reader-voted."],
      ["BBC Big Read", "The UK's best-loved novels, reader-voted."],
      ["ABC Radio National · Top 100 Books of the 21st Century", "An Australian listener countdown."],
      ["Will Durant · 100 Best Books for an Education", "A classic self-education reading plan."],
      ["Världsbiblioteket · 100 Best Books", "A Swedish literary poll and world-library canon."],
      ["BBC · 100 Novels That Shaped Our World", "Panel-selected books with broad cultural influence."],
      ["Goodreads · Best Books of All Time", "Reader-generated favorites with enormous participation."],
      ["Goodreads · Most Shelved Classics", "The classics readers return to and shelve most."],
      ["Bookshop.org · 100 Epic Reads of a Lifetime", "A popular and classic lifetime-reading mix."],
      ["Penguin Random House · Books Everyone Should Read", "Reader-driven recommendations from a major publisher."],
      ["100 Books to Read Before You Die", "A definitive-style classic reading challenge."],
      ["Harvard Classics · Five-Foot Shelf", "The influential educational bookshelf."],
      ["Great Books of the Western World", "The Adler and Hutchins Western canon."],
      ["Most Assigned Novels", "Books that appear most often on academic syllabi."],
      ["Most Popular Library Classics", "Long-running library favorites and staples."],
      ["National Reader Favorites", "A cross-national public-library and reader shelf."],
      ["Oprah's Book Club · Essential Reads", "Culturally significant selections from Oprah's club."],
      ["Amazon · Most Popular Books", "A sales and popularity-driven reading queue."],
      ["Locus · Science Fiction Canon", "A genre-focused science-fiction essentials shelf."],
      ["NPR · Top Science Fiction & Fantasy", "Reader-recommended speculative fiction."],
      ["Modern Library · 100 Best Nonfiction", "The nonfiction counterpart to the Modern Library novels."],
      ["Top 100 Historical Fiction", "A long queue of historical novels and period stories."],
      ["Top 100 Western Books", "Classic and modern Western reading."],
      ["Top 100 Mystery & Detective Books", "The most influential puzzles and investigations."],
      ["Top 100 Horror Books", "A broad literary and popular horror canon."],
      ["Top 100 Romance Books", "Enduring love stories and relationship novels."],
      ["Top 100 Biographies & Memoirs", "The lives and testimony that shaped readers."],
      ["Top 100 Philosophy Books", "Foundational works for a lifetime of thought."],
      ["Top 100 Children's Books", "Beloved books that reward rereading at every age."]
    ].map(([name, description], index) => [name, description, TOP_100_BOOKS, `source-${index}`]);

    const RECOMMENDED_LISTS = [
      ["TIME-Style Top 100 Books", "A deep all-time reading shelf—not a five-book sample.", TOP_100_BOOKS],
      ["Top 100 Novels of All Time", "A century-spanning novel queue for serious readers.", TOP_100_BOOKS],
      ["Top 100 Historical Reads", "The foundational novels, histories, memoirs, and biographies.", TOP_100_BOOKS],
      ["Top 100 Western Books", "A long-form Western reading shelf across classic and modern works.", TOP_100_BOOKS],
      ["Top 100 Science Fiction Books", "A full science-fiction reading queue from Wells to today.", TOP_100_BOOKS],
      ...SOURCE_RECOMMENDATIONS,
      ["Best Books of All Time", "A broad canon of enduring fiction and nonfiction.", ["Middlemarch", "The Great Gatsby", "Beloved", "War and Peace", "The Republic"]],
      ["Best Novels of All Time", "The essential novel canon across centuries.", ["Don Quixote", "Anna Karenina", "Middlemarch", "Ulysses", "One Hundred Years of Solitude"]],
      ["Best Nonfiction of All Time", "Landmark ideas, history, science, and memoir.", ["The Republic", "The Histories", "On the Origin of Species", "Silent Spring", "The Diary of a Young Girl"]],
      ["Best American Books", "Foundational American novels and voices.", ["Moby-Dick", "The Great Gatsby", "Invisible Man", "Beloved", "The Grapes of Wrath"]],
      ["Best British Books", "The British literary canon, from Austen to Woolf.", ["Pride and Prejudice", "Jane Eyre", "Middlemarch", "Mrs Dalloway", "Hamlet"]],
      ["Best World Literature", "Enduring books from around the world.", ["The Iliad", "The Divine Comedy", "The Tale of Genji", "The Brothers Karamazov", "One Hundred Years of Solitude"]],
      ["Best Books of the 20th Century", "The defining books of the modern century.", ["In Search of Lost Time", "The Sound and the Fury", "The Magic Mountain", "1984", "The Name of the Rose"]],
      ["Best Books of the 21st Century", "A high-signal contemporary reading shelf.", ["The Road", "2666", "The Brief Wondrous Life of Oscar Wao", "The Overstory", "The Underground Railroad"]],
      ["Best Short Books", "Canonical books you can finish in a weekend.", ["The Little Prince", "The Death of Ivan Ilyich", "The Stranger", "The Metamorphosis", "The Old Man and the Sea"]],
      ["Best Long Books", "Big, immersive novels worth the commitment.", ["The Count of Monte Cristo", "Les Misérables", "War and Peace", "The Lord of the Rings", "Infinite Jest"]],
      ["Best Debut Novels", "First novels that announced major voices.", ["Frankenstein", "The Bell Jar", "The God of Small Things", "The Secret History", "The Kite Runner"]],
      ["Best Science Fiction", "The essential science-fiction canon.", ["The War of the Worlds", "Brave New World", "Dune", "The Left Hand of Darkness", "Snow Crash"]],
      ["Best Fantasy", "The fantasy books that shaped the genre.", ["The Hobbit", "The Lord of the Rings", "A Wizard of Earthsea", "The Last Unicorn", "A Game of Thrones"]],
      ["Best Mystery Novels", "Unforgettable puzzles, detectives, and crimes.", ["The Murder of Roger Ackroyd", "The Hound of the Baskervilles", "The Big Sleep", "The Name of the Rose", "The Girl with the Dragon Tattoo"]],
      ["Best Horror Books", "The essential canon of literary horror.", ["Dracula", "Frankenstein", "The Haunting of Hill House", "The Exorcist", "The Shining"]],
      ["Best Romance Novels", "Enduring stories about love and longing.", ["Pride and Prejudice", "Jane Eyre", "Wuthering Heights", "Persuasion", "Love in the Time of Cholera"]],
      ["Best Biographies", "Lives that illuminate character, power, and history.", ["The Power Broker", "Long Walk to Freedom", "The Autobiography of Malcolm X", "Steve Jobs", "Alexander Hamilton"]],
      ["Best Memoirs", "Personal testimony with lasting literary power.", ["Night", "The Year of Magical Thinking", "Educated", "The Glass Castle", "When Breath Becomes Air"]],
      ["Best Philosophy Books", "Foundational works for thinking about life.", ["Meditations", "The Republic", "Nicomachean Ethics", "The Prince", "Being and Time"]],
      ["Best Poetry Books", "Poetry collections and epics that changed the form.", ["The Iliad", "The Divine Comedy", "Leaves of Grass", "The Waste Land", "The Complete Poems"]]
    ].map(([name, description, titles], index) => ({ id: `recommended-${index}`, name, description, titles }));

    // ── Curated shelves for every catalog ──────────────────────────────────
    // Titles are compared the way the web UI always has: lowercase, alphanumerics
    // only, so "Sapiens: A Brief History" and "sapiens a brief history" agree.
    function listTitleKey(value) {
      return String(value || "").toLowerCase().replace(/[^a-z0-9]+/g, "");
    }

    // The book shelves above were the only curated data in Sonder, and they
    // only ever matched ebooks and audiobooks. Nothing about a list is
    // book-specific, so the same engine now also drives movies, shows, and
    // documentaries: each list declares which media kinds it can match, and
    // each entry is either "Title" or "Title (Year)".
    //
    // The year is what makes a film list trustworthy. "The Thing" is both a
    // 1982 Carpenter film and a 2011 prequel, "Dune" spans 1984 and 2021, and
    // "It" spans every decade. An entry that names a year only matches a
    // catalog entry carrying the same year; an entry without one stays
    // title-only, which is all that genre shelves ever needed.
    //
    // These are selections, not complete canons, and each list says so. The
    // goal is a shelf that is mostly present in a real library: a list you
    // already half own is what makes the remaining half a queue.
    const CURATED_KINDS = {
      book: ["ebook", "audiobook"],
      movie: ["movie"],
      tv: ["tvShow"],
      doc: ["documentary"],
    };

    const CURATED_MOVIE_LISTS = [
      ["Greatest Movies of All Time", "A cross-decade selection from the major critics' polls — the films the canon cannot be argued without.", "movie", [
        "Metropolis (1927)", "M (1931)", "Bicycle Thieves (1948)", "Citizen Kane (1941)", "Rashomon (1950)",
        "Tokyo Story (1953)", "Seven Samurai (1954)", "The Searchers (1956)", "On the Waterfront (1954)", "Sunset Boulevard (1950)",
        "Vertigo (1958)", "Yojimbo (1961)", "Lawrence of Arabia (1962)", "8½ (1963)", "2001: A Space Odyssey (1968)",
        "Psycho (1960)", "The Godfather (1972)", "The Godfather Part II (1974)", "Jaws (1975)", "Taxi Driver (1976)",
        "Annie Hall (1977)", "Apocalypse Now (1979)", "Raging Bull (1980)", "Chinatown (1974)", "Blade Runner (1982)",
        "Rear Window (1954)", "Casablanca (1942)", "Modern Times (1936)", "Some Like It Hot (1959)", "Singin' in the Rain (1952)",
        "The Apartment (1960)", "Star Wars (1977)", "The Shining (1980)", "Das Boot (1981)", "Ran (1985)",
        "Cinema Paradiso (1988)", "Goodfellas (1990)", "Schindler's List (1993)", "Pulp Fiction (1994)", "Fight Club (1999)",
        "Good Will Hunting (1997)", "In the Mood for Love (2000)", "Mulholland Drive (2001)", "There Will Be Blood (2007)",
        "Get Out (2017)", "La Haine (1995)", "Oldboy (2003)", "Solaris (1972)", "Stalker (1979)",
        "Portrait of a Lady on Fire (2019)", "Parasite (2019)", "Platoon (1986)", "Amadeus (1984)", "The Untouchables (1987)",
        "Fargo (1996)", "Titanic (1997)", "No Country for Old Men (2007)", "Whiplash (2014)", "Moonlight (2016)"
      ]],
      ["American Cinema Essentials", "A selection from AFI's 100 American Classics and the wider canon of American film.", "movie", [
        "Citizen Kane (1941)", "The Grapes of Wrath (1940)", "Casablanca (1942)", "Gone with the Wind (1939)", "The Maltese Falcon (1941)",
        "The Bridge on the River Kwai (1957)", "Ben-Hur (1959)", "Singin' in the Rain (1952)", "On the Waterfront (1954)", "Some Like It Hot (1959)",
        "The Apartment (1960)", "West Side Story (1961)", "The Godfather (1972)", "The Godfather Part II (1974)", "Jaws (1975)",
        "Rocky (1976)", "Annie Hall (1977)", "Star Wars (1977)", "Apocalypse Now (1979)", "Raging Bull (1980)",
        "E.T. the Extra-Terrestrial (1982)", "Terms of Endearment (1983)", "Amadeus (1984)", "Platoon (1986)", "The Untouchables (1987)",
        "Rain Man (1988)", "Goodfellas (1990)", "The Silence of the Lambs (1991)", "Schindler's List (1993)", "Forrest Gump (1994)",
        "Braveheart (1995)", "Pulp Fiction (1994)", "Fargo (1996)", "Titanic (1997)", "Saving Private Ryan (1998)",
        "The Matrix (1999)", "Gladiator (2000)", "A Beautiful Mind (2001)", "Chicago (2002)", "Million Dollar Baby (2004)",
        "Crash (2005)", "The Departed (2006)", "No Country for Old Men (2007)", "Slumdog Millionaire (2008)", "The Hurt Locker (2008)",
        "The Social Network (2010)", "The Artist (2011)", "Argo (2012)", "12 Years a Slave (2013)", "Birdman (2014)",
        "The Revenant (2015)", "Moonlight (2016)", "La La Land (2016)", "Get Out (2017)", "Green Book (2018)",
        "Parasite (2019)", "Nomadland (2020)", "Everything Everywhere All at Once (2022)"
      ]],
      ["The Criterion Essentials", "Restorations and filmmaker spotlights that made the standard for serious home viewing.", "movie", [
        "Bicycle Thieves (1948)", "Pather Panchali (1955)", "Tokyo Story (1953)", "Rashomon (1950)", "Seven Samurai (1954)",
        "Yojimbo (1961)", "High and Low (1963)", "Persona (1966)", "Andrei Rublev (1966)", "Stalker (1979)",
        "Apocalypse Now (1979)", "The Godfather (1972)", "Vertigo (1958)", "Rear Window (1954)", "Sunset Boulevard (1950)",
        "Some Like It Hot (1959)", "The 400 Blows (1959)", "Breathless (1960)", "La Dolce Vita (1960)", "The Saragossa Manuscript (1965)",
        "The Battle of Algiers (1966)", "Black Girl (1966)", "In the Mood for Love (2000)", "Beau Travail (1999)", "Wanda (1970)",
        "The Last Emperor (1987)", "Fitzcarraldo (1982)", "Paris, Texas (1984)", "Wings of Desire (1987)", "Cleo from 5 to 7 (1962)",
        "Last Year at Marienbad (1971)", "The 39 Steps (1935)", "Kind Hearts and Coronets (1949)", "The Ascent (1977)", "Black Orpheus (1959)"
      ]],
      ["Science Fiction and Space", "The ideas, machines, and first contacts that keep the genre moving forward.", "movie", [
        "2001: A Space Odyssey (1968)", "Solaris (1972)", "Star Wars (1977)", "Close Encounters of the Third Kind (1977)", "Alien (1979)",
        "Blade Runner (1982)", "The Thing (1982)", "E.T. the Extra-Terrestrial (1982)", "Tron (1982)", "The Terminator (1984)",
        "Back to the Future (1985)", "Aliens (1986)", "Total Recall (1990)", "Terminator 2: Judgment Day (1991)", "12 Monkeys (1995)",
        "The Fifth Element (1997)", "Gattaca (1997)", "The Matrix (1999)", "Primer (2004)", "Serenity (2005)",
        "District 9 (2009)", "Moon (2009)", "Inception (2010)", "Source Code (2011)", "Looper (2012)",
        "Gravity (2013)", "Her (2013)", "Under the Skin (2013)", "Interstellar (2014)", "Ex Machina (2014)",
        "Arrival (2016)", "Passengers (2016)", "Annihilation (2018)", "Upgrade (2018)", "The Martian (2015)",
        "Blade Runner 2049 (2017)", "Dune (2021)", "Everything Everywhere All at Once (2022)", "Nope (2022)", "Poor Things (2023)",
        "The Substance (2024)", "Coherence (2013)", "Sunshine (2007)", "Stalker (1979)", "Minority Report (2002)"
      ]],
      ["Crime, Noir and Thrillers", "Traps, cons, and moral pressure — the sharpest plot-driven films in the library.", "movie", [
        "The Maltese Falcon (1941)", "Double Indemnity (1944)", "The Big Sleep (1946)", "Out of the Past (1947)", "Laura (1944)",
        "Touch of Evil (1958)", "Vertigo (1958)", "The Manchurian Candidate (1962)", "Rosemary's Baby (1968)", "The French Connection (1971)",
        "Chinatown (1974)", "Jaws (1975)", "The Deer Hunter (1978)", "The Shining (1980)", "Blade Runner (1982)",
        "Scarface (1983)", "Prizzi's Honor (1985)", "The Untouchables (1987)", "Die Hard (1988)", "Goodfellas (1990)",
        "The Silence of the Lambs (1991)", "Cape Fear (1991)", "Basic Instinct (1992)", "Reservoir Dogs (1992)", "True Romance (1993)",
        "Léon: The Professional (1994)", "Pulp Fiction (1994)", "Fargo (1996)", "L.A. Confidential (1997)", "Heat (1995)",
        "The Usual Suspects (1995)", "Se7en (1995)", "Good Will Hunting (1997)", "Memento (2000)", "Snatch (2001)",
        "The Prestige (2006)", "Zodiac (2007)", "No Country for Old Men (2007)", "The Departed (2006)", "Collateral (2004)",
        "Prisoners (2013)", "Gone Girl (2014)", "The Town (2010)", "Drive (2011)", "Nightcrawler (2014)", "Argo (2012)"
      ]],
      ["Horror and Dread", "The genre's essentials, from practical effects to the films that made being scared respectable.", "movie", [
        "Night of the Living Dead (1968)", "The Omen (1976)", "The Wicker Man (1973)", "The Texas Chain Saw Massacre (1974)", "Jaws (1975)",
        "Alien (1979)", "The Shining (1980)", "The Evil Dead (1981)", "The Thing (1982)", "Poltergeist (1982)",
        "A Nightmare on Elm Street (1984)", "The Fly (1986)", "The Exorcist (1973)", "The Silence of the Lambs (1991)", "Misery (1990)",
        "Halloween (1978)", "The Blair Witch Project (1999)", "The Sixth Sense (1999)", "Final Destination (2000)", "Scream (1996)",
        "The Ring (2002)", "28 Days Later (2002)", "The Descent (2005)", "Ginger Snaps (2006)", "REC (2007)",
        "Let the Right One In (2008)", "Sinister (2012)", "The Conjuring (2013)", "It (2017)", "Hereditary (2018)",
        "Us (2019)", "Midsommar (2019)", "Get Out (2017)", "A Quiet Place (2018)", "The Lighthouse (2019)", "The Babadook (2014)",
        "The Witch (2015)", "It Follows (2014)", "Suspiria (1977)", "Possession (1981)", "The Others (2001)", "Coraline (2009)"
      ]],
      ["Animation and Family Favourites", "Hand-drawn milestones and modern studio peaks for the whole household.", "movie", [
        "Akira (1988)", "My Neighbor Totoro (1988)", "Grave of the Fireflies (1988)", "The Nightmare Before Christmas (1993)", "Aladdin (1992)",
        "Beauty and the Beast (1991)", "The Lion King (1994)", "Toy Story (1995)", "Mulan (1998)", "Princess Mononoke (1997)",
        "The Iron Giant (1999)", "Spirited Away (2001)", "Shrek (2001)", "Finding Nemo (2003)", "The Triplets of Belleville (2003)",
        "The Incredibles (2004)", "Howl's Moving Castle (2004)", "Ratatouille (2007)", "Ponyo (2008)", "WALL-E (2008)",
        "Up (2009)", "Coraline (2009)", "The Secret of Kells (2009)", "Toy Story 3 (2010)", "Ernest & Celestine (2012)",
        "Frozen (2013)", "Song of the Sea (2014)", "Inside Out (2015)", "Moana (2016)", "Your Name (2016)",
        "Kubo and the Two Strings (2016)", "Coco (2017)", "The Boss Baby (2017)", "Wolfwalkers (2020)", "Turning Red (2022)",
        "Belle (2021)", "The Boy and the Heron (2023)", "Only Yesterday (1991)", "Whisper of the Heart (1995)", "Persepolis (2007)",
        "When Marnie Was There (2014)", "The Red Turtle (2016)", "The Willoughbys (2020)", "Poupelle of Chimney Town (2020)"
      ]],
      ["Recent Award Winners", "Best Picture and Best Director winners of the last fifteen years.", "movie", [
        "The Departed (2006)", "No Country for Old Men (2007)", "Slumdog Millionaire (2008)", "The Hurt Locker (2009)", "The King's Speech (2010)",
        "The Artist (2011)", "Argo (2012)", "12 Years a Slave (2013)", "Birdman (2014)", "The Revenant (2015)",
        "Spotlight (2015)", "Moonlight (2016)", "La La Land (2016)", "The Shape of Water (2017)", "Get Out (2017)",
        "Black Panther (2018)", "Green Book (2018)", "Parasite (2019)", "Nomadland (2020)", "CODA (2021)",
        "Everything Everywhere All at Once (2022)", "Oppenheimer (2023)", "Anora (2024)", "The Substance (2024)", "Sinners (2025)"
      ]]
    ];

    const CURATED_TV_LISTS = [
      ["Greatest Television of All Time", "The dramatic canon: the shows that made prestige television a form.", "tv", [
        "The Twilight Zone (1959)", "NYPD Blue (1993)", "The X-Files (1993)", "Oz (1997)", "The Sopranos (1999)",
        "The West Wing (1999)", "24 (2001)", "Band of Brothers (2001)", "Six Feet Under (2001)", "The Wire (2002)",
        "The Shield (2002)", "Arrested Development (2003)", "Deadwood (2004)", "Rome (2005)", "The Office (2005)",
        "Damages (2007)", "Breaking Bad (2008)", "Boston Legal (2004)", "Fargo (2014)", "Halt and Catch Fire (2014)",
        "Justified (2010)", "Mad Men (2007)", "Boardwalk Empire (2010)", "Downton Abbey (2010)", "The Leftovers (2014)",
        "Bates Motel (2013)", "The Americans (2013)", "Ozark (2017)", "Succession (2018)", "The Marvelous Mrs. Maisel (2018)",
        "Mr. Robot (2015)", "Better Call Saul (2015)", "The Expanse (2015)", "Twin Peaks (1990)", "The Simpsons (1989)",
        "Chernobyl (2019)", "Andor (2022)", "Severance (2022)", "Fleabag (2016)", "The Bear (2022)"
      ]],
      ["Prestige and Period Drama", "Corsets, empires, matriarchs, and the slow burn of a serious period production.", "tv", [
        "Downton Abbey (2010)", "Mad Men (2007)", "The Crown (2016)", "Boardwalk Empire (2010)", "The Tudors (2007)",
        "Wolf Hall (2015)", "The Knick (2015)", "The Gilded Age (2022)", "Victoria (2016)", "Poldark (2015)",
        "The White Princess (2017)", "Peaky Blinders (2013)", "Narcos (2015)", "The Man in the High Castle (2015)", "Brave New World (2020)",
        "Chernobyl (2019)", "The Terror (2018)", "The Last Kingdom (2015)", "Black Sails (2014)", "Versailles (2015)",
        "Outlander (2014)", "The Alienist (2018)", "Normal People (2020)", "Anne with an E (2017)"
      ]],
      ["Comedy Worth Rewatching", "Sitcoms and dramedies with a long shelf life and a quotable line per episode.", "tv", [
        "Seinfeld (1989)", "The Simpsons (1989)", "Friends (1994)", "That '70s Show (1998)", "Futurama (1999)",
        "Black Books (2000)", "Curb Your Enthusiasm (2000)", "Arrested Development (2003)", "Peep Show (2003)", "The Office (2005)",
        "It's Always Sunny in Philadelphia (2005)", "30 Rock (2006)", "Community (2009)", "Parks and Recreation (2009)", "Modern Family (2009)",
        "Sherlock (2010)", "Silicon Valley (2014)", "BoJack Horseman (2014)", "Brooklyn Nine-Nine (2013)", "Schitt's Creek (2015)",
        "The Good Place (2016)", "Insecure (2016)", "Derry Girls (2016)", "Detectorists (2014)", "Veep (2012)",
        "Ramy (2019)", "Abbott Elementary (2021)", "The Bear (2022)", "Atlanta (2018)", "Shrill (2019)",
        "Ted Lasso (2020)", "Mythic Quest (2019)", "Never Have I Ever (2020)", "Rick and Morty (2013)", "Beef (2023)",
        "Raising Dion (2019)", "Cobra Kai (2018)", "GLOW (2017)", "Baskets (2016)", "Nathan for You (2013)"
      ]],
      ["Speculative and Genre Series", "The shows that turned genre television into a delivery system for ideas.", "tv", [
        "The X-Files (1993)", "Buffy the Vampire Slayer (1997)", "Firefly (2002)", "Lost (2004)", "Battlestar Galactica (2004)",
        "Doctor Who (2005)", "Supernatural (2005)", "Fringe (2008)", "True Blood (2008)", "The Vampire Diaries (2009)",
        "Game of Thrones (2011)", "Person of Interest (2011)", "Black Mirror (2011)", "The Walking Dead (2010)", "Stranger Things (2016)",
        "Westworld (2016)", "The Handmaid's Tale (2017)", "Dark (2017)", "The Expanse (2015)", "The Orville (2017)",
        "Better Call Saul (2015)", "Mr. Robot (2015)", "Legion (2017)", "Maniac (2018)", "Devs (2020)",
        "Foundation (2021)", "The Wheel of Time (2021)", "Severance (2022)", "Silo (2022)", "The Last of Us (2023)",
        "Fallout (2024)", "3 Body Problem (2024)", "Shōgun (2024)", "The Sympathizer (2024)", "Ripley (2024)",
        "Scavengers Reign (2023)", "His Dark Materials (2019)", "Arcane (2021)", "Pantheon (2022)", "Blue Eye Samurai (2023)", "Andor (2022)"
      ]]
    ];

    const CURATED_DOC_LISTS = [
      ["Greatest Documentaries", "Non-fiction features that made the form feel like a category worth chasing.", "doc", [
        "Nanook of the North (1922)", "Häxan (1922)", "Man with a Movie Camera (1929)", "Let There Be Light (1946)", "Tokyo Olympiad (1961)",
        "Titicut Follies (1967)", "Salesman (1969)", "7 Up (1969)", "Grey Gardens (1975)", "Harlan County War (1976)",
        "The Last Waltz (1978)", "The Thin Blue Line (1988)", "Paris Is Burning (1990)", "Hoop Dreams (1994)", "Crumb (1994)",
        "When We Were Kings (1996)", "The Last Days (1998)", "One Day in September (1999)", "Spellbound (2002)", "Born into Brothels (2004)",
        "Grizzly Man (2005)", "March of the Penguins (2005)", "Man on Wire (2008)", "The Cove (2009)", "Winter on Fire (2012)",
        "Searching for Sugar Man (2012)", "The Act of Killing (2012)", "20 Feet from Stardom (2013)", "Citizenfour (2014)", "Amy (2015)",
        "13th (2016)", "I Am Not Your Negro (2016)", "American Factory (2019)", "The Cave (2019)", "Crip Camp (2019)",
        "Knife Skills (2019)", "The Social Dilemma (2020)", "Collective (2020)", "The Reason I Jump (2020)", "My Octopus Teacher (2020)",
        "The Dissident (2020)", "All That Breathes (2021)", "Fire of Love (2022)", "The Territory (2022)", "OJ: Made in America (2016)",
        "Summer of Soul (2021)", "The Greatest Night in Pop (2024)", "Get Back (2021)", "Super/Man (2022)"
      ]],
      ["Nature, Science and Space", "Oceans, planets, and animals — the nonfiction shelf that makes a living room feel bigger.", "doc", [
        "Nanook of the North (1922)", "Microcosmos (1996)", "Winged Migration (2001)", "The Blue Planet (2001)", "Volcanoes of the Deep Sea (2003)",
        "Deep Sea 3D (2006)", "Earth (2007)", "Oceans (2009)", "The Planets (2009)", "Island (2011)",
        "African Cats (2011)", "Chasing Ice (2012)", "Cave of Forgotten Dreams (2010)", "The Elephant Queen (2018)", "The Hunt (2015)",
        "Planet Earth II (2016)", "Deep Blue (2017)", "The Biggest Little Farm (2018)", "Honeyland (2019)", "Free Solo (2018)",
        "Night on Earth (2020)", "A Life on Our Planet (2020)", "My Octopus Teacher (2020)", "The Elephant Whisperers (2022)", "Sea Rex (2020)",
        "The Last Honey Hunter (2017)", "Antarctica (2015)", "The Hottest August (2019)", "Fire (2015)", "The Story of Plastic (2018)",
        "Wildcat (2022)", "Snow (2023)", "The Secret Life of Elephants (2018)", "Cosmos: A Spacetime Odyssey (2014)", "The Planets (2019)"
      ]],
      ["History, War and Power", "Archives, tribunals, and turning points told by people who were in the room.", "doc", [
        "Hearts and Minds (1974)", "Shoah (1985)", "The Thin Blue Line (1988)", "The War (1994)", "The War Rooms (1993)",
        "The Longest Day (1962)", "Sahara (1943)", "The Last Days (1998)", "The Fog of War (2003)", "The Lookout (2009)",
        "The Gatekeepers (2012)", "How to Survive a Plague (2012)", "Winter on Fire (2012)", "The Tillman Story (2010)", "The Queen of Versailles (2012)",
        "The 5th Estate (2013)", "Citizenfour (2014)", "The Program (2015)", "The Panama Papers (2016)", "Into the Whirlwind (2016)",
        "Icarus (2017)", "Last Men in Aleppo (2017)", "The Price of Everything (2018)", "American Factory (2019)", "The Reason I Jump (2020)",
        "Collective (2020)", "Navalny (2022)", "The 12th Victim (2020)", "Attica (2021)", "The Commandant's Shadow (2023)",
        "The Territory (2022)", "Fire of Love (2022)", "The Dissident (2020)", "Crip Camp (2019)", "OJ: Made in America (2016)"
      ]],
      ["Music, Art and Performance", "Concert films, studio documentaries, and the people behind the records.", "doc", [
        "Let There Be Light (1946)", "Gimme Shelter (1970)", "Wattstax (1973)", "The Last Waltz (1978)", "The Decline of Western Civilization (1978)",
        "Stop Making Sense (1983)", "Kurt & Courtney (1997)", "Buena Vista Social Club (1999)", "Standing in the Shadows of Motown (2002)", "Tin Drum (2008)",
        "The Yes Men (2003)", "Moonage Daydream (2012)", "Searching for Sugar Man (2012)", "20 Feet from Stardom (2013)", "Amy (2015)",
        "The Beatles: Eight Days a Week (2016)", "The Myth of Fingerprints (2018)", "The Last Movie Painter (2019)", "Be Water (2020)", "Stardust (2020)",
        "Summer of Soul (2021)", "The Lady and the Dale (2021)", "Catching Fire: The Story of Anita Pallenberg (2022)", "The Greatest Night in Pop (2024)", "Get Back (2021)",
        "Serge Gainsbourg: Gainsbourg et ses complices (2010)", "Jaco (2015)", "Listen to Me Marlon (2015)", "Chasing Great (2016)", "The Velvet Underground (2021)", "I Am Divine (2019)"
      ]]
    ];

    // Curated entries are "Title" or "Title (Year)". Parsing is memoized because
    // dozens of book shelves share one 100-title array.
    const CURATED_TITLE_YEAR = /^(.+?)\s*\((\d{4})\)\s*$/;
    const curatedEntryCache = new Map();
    function parseCuratedEntry(entry) {
      const value = String(entry || "");
      const hit = curatedEntryCache.get(value);
      if (hit) return hit;
      const match = CURATED_TITLE_YEAR.exec(value);
      const parsed = match
        ? { title: match[1].trim(), year: Number(match[2]) }
        : { title: value, year: 0 };
      curatedEntryCache.set(value, parsed);
      return parsed;
    }

    // One registry for every catalog. Books keep their historical `titles`
    // field so the existing shelf UI keeps working; every list also exposes
    // parsed `entries` and the `kinds` it can match against. `signature`
    // identifies shelves that hold identical titles, which is how the many
    // differently-named "top 100 books" lists are recognised as one shelf.
    const curatedSignature = entries => entries.map(entry => `${listTitleKey(entry.title)}|${entry.year || ""}`).join("");
    const CURATED_LISTS = [
      ...RECOMMENDED_LISTS.map(list => ({
        id: list.id, name: list.name, description: list.description,
        kinds: CURATED_KINDS.book, titles: list.titles,
        entries: list.titles.map(parseCuratedEntry),
      })),
      ...[...CURATED_MOVIE_LISTS, ...CURATED_TV_LISTS, ...CURATED_DOC_LISTS].map(
        ([name, description, kind, titles], index) => ({
          id: `curated-${index}`, name, description, kinds: CURATED_KINDS[kind],
          titles, entries: titles.map(parseCuratedEntry),
        })),
    ].map(list => ({ ...list, signature: curatedSignature(list.entries) }));

    // Which media kinds does this tab browse? Drives both the curated shelf
    // picker and the list add-row search so neither offers an item the user
    // cannot see.
    function tabCatalogKinds(tab = activeTab) {
      return (kindByTab[tab] ? [kindByTab[tab]] : null) ?? [];
    }

    function curatedListsForKinds(kinds) {
      if (!kinds || !kinds.length) return [];
      return CURATED_LISTS.filter(list => list.kinds.some(kind => kinds.includes(kind)));
    }

    const { api, escapeHTML, formatTime } = window.Sonder;

    let libraryLayout = "rails";
    let hideEmptyLibraries = true;
    function applyLibraryLayout(value) {
      libraryLayout = String(value || "rails").toLowerCase() === "classic" ? "classic" : "rails";
      if (typeof document === "undefined") return;
      if (document.documentElement) document.documentElement.dataset.libraryLayout = libraryLayout;
      if (document.body) document.body.dataset.libraryLayout = libraryLayout;
    }

    function applyTheme(preset) {
      const theme = preset || "earthy";
      document.documentElement.dataset.theme = theme;
      document.body.dataset.theme = theme;
    }

    // Query/element helpers tolerate a missing DOM so the pure logic in this
    // file can be evaluated in tests without a browser.
    const $ = sel => (typeof document === "undefined" ? null : document.querySelector(sel));
    function on(selector, event, handler) {
      const el = $(selector);
      if (el && typeof el.addEventListener === "function") el.addEventListener(event, handler);
    }

    // --- built-in player ---------------------------------------------------
    // One persistent media element, kept outside every re-rendered region, so
    // audio keeps playing while you browse. The controller docks to the bottom
    // and drives both audio and video; video can be collapsed to audio-only.
    const DIRECT_AUDIO = new Set(["mp3", "m4a", "m4b", "aac", "flac", "wav", "ogg"]);
    const DIRECT_VIDEO = new Set(["mp4", "m4v", "mov", "webm"]);
    const VIDEO_KINDS = new Set(["movie", "tvShow", "documentary"]);

    // playbackPlan decides how an item plays in the built-in player: a direct
    // byte-range stream when the browser can decode the container, otherwise
    // the server's on-the-fly fMP4 transcode (mkv, avi, ...). Audiobook
    // containers such as .m4b are MP4 audio and play directly when the browser
    // supports their probed codec (including Opus in MP4).
    function playbackPlan(item) {
      const format = String(item.format || "").toLowerCase();
      const kind = String(item.kind || "");
      if (DIRECT_AUDIO.has(format)) return { mode: "audio", url: "/stream/" + item.id };
      if (DIRECT_VIDEO.has(format)) return { mode: "video", url: "/stream/" + item.id };
      if (VIDEO_KINDS.has(kind)) return { mode: "video", url: "/stream/" + item.id + "?transcode=1" };
      return null;
    }

    function mediaLabel(mode) { return mode === "audio" ? "Audio" : "Video"; }

    let nowPlayingItem = null;
    let nowPlayingMode = "audio";
    let npSeeking = false;
    let npLastSaved = 0;

    function npMedia() { return $("#npMedia"); }

    function startPlaybackById(id) {
      const item = items.find(candidate => candidate.id === id);
      if (!item) return;
      if (nowPlayingItem && nowPlayingItem.id === id) { togglePlay(); return; }
      const record = progressFor(id);
      startPlayback(item, record && record.seconds > 5 ? record.seconds : 0);
    }

    function startPlayback(item, resumeAt = 0) {
      const plan = playbackPlan(item);
      const media = npMedia();
      if (!plan || !media) return;
      saveProgress(true);

      nowPlayingItem = item;
      nowPlayingMode = plan.mode;
      npLastSaved = 0;
      npSeeking = false;

      media.src = api(plan.url);
      media.playbackRate = Number($("#npRate")?.value) || 1;
      media.onerror = () => {
        const codecs = Array.isArray(item.probedAudioCodecs) ? item.probedAudioCodecs : [];
        const hasOpus = codecs.some(codec => String(codec).toLowerCase() === "opus");
        const opusSupport = typeof document !== "undefined"
          ? document.createElement("audio").canPlayType('audio/mp4; codecs="opus"') : "";
        if (hasOpus && !opusSupport) {
          const status = $("#npStatus");
          if (status) status.textContent = "This browser cannot decode Opus in MP4. Try the iOS app or a browser with Opus-in-MP4 support.";
        }
      };

      const host = $("#nowPlaying");
      if (host) {
        host.hidden = false;
        // Video starts visible; audio never shows a video surface.
        host.classList.toggle("np-audio-mode", plan.mode === "audio");
      }
      if (typeof document !== "undefined") document.body.classList.add("np-visible");
      const expand = $("#npExpand");
      if (expand) {
        expand.hidden = plan.mode === "audio";
        expand.textContent = "⤡";
        expand.setAttribute("aria-label", "Hide video and keep playing");
        expand.setAttribute("aria-pressed", "true");
      }

      const status = $("#npStatus");
      if (status) {
        status.textContent = plan.mode === "audio"
          ? "Plays in the background — these controls stay while you browse."
          : "";
      }
      renderNowPlaying();
      updateMediaSession(item);

      media.addEventListener("loadedmetadata", () => {
        const duration = media.duration || 0;
        if (resumeAt > 5 && resumeAt < Math.max(duration - 8, 0)) media.currentTime = resumeAt;
        media.play().catch(() => {});
        onTimeUpdate();
      }, { once: true });
    }

    function togglePlay() {
      const media = npMedia();
      if (!nowPlayingItem || !media) return;
      if (media.paused) media.play().catch(() => {}); else media.pause();
    }

    function skipBy(delta) {
      const media = npMedia();
      if (!nowPlayingItem || !media || !Number.isFinite(media.duration)) return;
      media.currentTime = Math.min(Math.max(media.currentTime + delta, 0), media.duration || 0);
      onTimeUpdate();
    }

    function stopPlayback() {
      const media = npMedia();
      saveProgress(true);
      if (media) {
        media.pause();
        media.removeAttribute("src");
        media.load();
      }
      nowPlayingItem = null;
      const host = $("#nowPlaying");
      if (host) host.hidden = true;
      if (typeof document !== "undefined") document.body.classList.remove("np-visible");
      if (typeof navigator !== "undefined" && "mediaSession" in navigator) {
        navigator.mediaSession.metadata = null;
        navigator.mediaSession.playbackState = "none";
      }
      const status = $("#npStatus");
      if (status) status.textContent = "";
      render();
    }

    // Progress is written on a 15s cadence, on pause/ended, and when the page
    // is hidden or unloaded, so background listening still records position.
    function saveProgress(force = false) {
      const media = npMedia();
      const item = nowPlayingItem;
      if (!item || !media || !media.duration || !Number.isFinite(media.currentTime)) return;
      if (!force && Math.abs(media.currentTime - npLastSaved) < 15) return;
      npLastSaved = media.currentTime;
      const payload = { seconds: media.currentTime, duration: media.duration };
      progressByID.set(item.id, {
        itemID: item.id, seconds: payload.seconds, duration: payload.duration,
        updatedAt: new Date().toISOString(),
      });
      fetch(api("/api/progress/" + item.id), {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload), keepalive: true,
      }).catch(() => {});
    }

    function renderNowPlaying() {
      const item = nowPlayingItem;
      const host = $("#nowPlaying");
      if (!item) { if (host) host.hidden = true; return; }

      const title = $("#npTitle");
      if (title) {
        title.textContent = item.showTitle && item.seasonNumber != null
          ? `${item.showTitle} — ${item.title}` : item.title;
      }
      const sub = $("#npSub");
      if (sub) {
        sub.textContent = [kindLabel(item.kind), item.author || item.studio || "", item.year || ""]
          .filter(Boolean).join(" • ");
      }
      const art = $("#npArt");
      if (art) {
        const src = item.posterURL ? api(item.posterURL) : "";
        art.innerHTML = src ? `<img src="${escapeHTML(src)}" alt="">` : "♪";
        art.setAttribute("aria-label", `Now playing: ${item.title}`);
      }
      onPlayStateChange();
    }

    function onPlayStateChange() {
      const media = npMedia();
      if (!media) return;
      const playing = !media.paused && !media.ended;
      const button = $("#npPlayPause");
      if (button) {
        button.textContent = playing ? "❚❚" : "▶";
        button.setAttribute("aria-label", playing ? "Pause" : "Play");
        button.setAttribute("aria-pressed", playing ? "true" : "false");
      }
      if (typeof navigator !== "undefined" && "mediaSession" in navigator) {
        navigator.mediaSession.playbackState = playing ? "playing" : "paused";
      }
    }

    function onTimeUpdate() {
      const media = npMedia();
      if (!media) return;
      const duration = media.duration || 0;
      const current = media.currentTime || 0;
      const seek = $("#npSeek");
      if (seek && !npSeeking && duration > 0) seek.value = String(Math.round((current / duration) * 1000));
      const cur = $("#npCur"); if (cur) cur.textContent = formatTime(current);
      const dur = $("#npDur"); if (dur) dur.textContent = formatTime(duration);
      updatePositionState();
      saveProgress(false);
    }

    // Media Session gives OS/lock-screen controls and keeps playback alive in
    // the background where the platform supports it.
    function updateMediaSession(item) {
      if (typeof navigator === "undefined" || !("mediaSession" in navigator)) return;
      const artwork = item.posterURL
        ? [{ src: api(item.posterURL), sizes: "512x512", type: "image/jpeg" }]
        : [];
      try {
        navigator.mediaSession.metadata = new MediaMetadata({
          title: item.title || "",
          artist: item.author || item.studio || "",
          album: item.showTitle || "",
          artwork,
        });
      } catch { /* MediaMetadata unsupported */ }
      const media = npMedia();
      const handler = (action, fn) => {
        try { navigator.mediaSession.setActionHandler(action, fn); } catch { /* unsupported action */ }
      };
      handler("play", () => media && media.play());
      handler("pause", () => media && media.pause());
      handler("seekbackward", details => skipBy(-((details && details.seekOffset) || 30)));
      handler("seekforward", details => skipBy((details && details.seekOffset) || 30));
      handler("seekto", details => {
        if (media && details && details.seekTime != null) media.currentTime = details.seekTime;
      });
      handler("stop", () => stopPlayback());
    }

    function updatePositionState() {
      if (typeof navigator === "undefined" || !("mediaSession" in navigator)) return;
      const session = navigator.mediaSession;
      if (!session.setPositionState) return;
      const media = npMedia();
      if (!media || !media.duration || !Number.isFinite(media.duration) || media.duration <= 0) return;
      try {
        session.setPositionState({
          duration: media.duration,
          playbackRate: media.playbackRate || 1,
          position: Math.min(Math.max(media.currentTime || 0, 0), media.duration),
        });
      } catch { /* transient state, safe to skip */ }
    }

    function onDocument(event, handler) {
      if (typeof document !== "undefined" && typeof document.addEventListener === "function") {
        document.addEventListener(event, handler);
      }
    }

    // Wire the controller once; `on` tolerates a missing DOM so the pure logic
    // above stays testable outside a browser.
    on("#npPlayPause", "click", () => togglePlay());
    on("#npBack", "click", () => skipBy(-30));
    on("#npFwd", "click", () => skipBy(30));
    on("#npClose", "click", () => stopPlayback());
    on("#npArt", "click", () => { if (nowPlayingItem) openDetail(nowPlayingItem.id, true); });
    on("#npExpand", "click", () => {
      const host = $("#nowPlaying");
      const button = $("#npExpand");
      if (!host) return;
      const collapsed = host.classList.toggle("np-audio-mode");
      if (button) {
        button.textContent = collapsed ? "⤢" : "⤡";
        button.setAttribute("aria-label", collapsed ? "Show video" : "Hide video and keep playing");
        button.setAttribute("aria-pressed", collapsed ? "false" : "true");
      }
    });
    on("#npRate", "change", event => {
      const media = npMedia();
      if (media) media.playbackRate = Number(event.target.value) || 1;
      updatePositionState();
    });
    on("#npSeek", "input", event => {
      npSeeking = true;
      const media = npMedia();
      if (media && media.duration) {
        media.currentTime = (Number(event.target.value) / 1000) * media.duration;
        const cur = $("#npCur");
        if (cur) cur.textContent = formatTime(media.currentTime);
      }
    });
    on("#npSeek", "change", () => { npSeeking = false; });
    on("#npMedia", "timeupdate", () => onTimeUpdate());
    on("#npMedia", "play", () => onPlayStateChange());
    on("#npMedia", "pause", () => { onPlayStateChange(); saveProgress(true); });
    on("#npMedia", "ended", () => { onPlayStateChange(); saveProgress(true); });
    on("#npMedia", "loadedmetadata", () => onTimeUpdate());
    on("#npMedia", "error", () => {
      const status = $("#npStatus");
      if (status) status.textContent = "This stream could not be played here. Try “Open stream URL” instead.";
    });
    onDocument("keydown", event => {
      if (!nowPlayingItem) return;
      const target = event.target;
      const typing = target && (target.tagName === "INPUT" || target.tagName === "SELECT" ||
        target.tagName === "TEXTAREA" || target.isContentEditable);
      if (typing) return;
      if (event.code === "Space" || event.key === " ") { event.preventDefault(); togglePlay(); }
      else if (event.key === "ArrowLeft") { event.preventDefault(); skipBy(-15); }
      else if (event.key === "ArrowRight") { event.preventDefault(); skipBy(15); }
    });
    onDocument("visibilitychange", () => {
      if (typeof document !== "undefined" && document.visibilityState === "hidden") saveProgress(true);
    });
    onDocument("pagehide", () => saveProgress(true));

    // --- metadata facets (genres / authors / narrators / studios) ----------
    // Facets are scoped to the items in the current tab, so movie genres and
    // book subjects never share one list. Modes with no values are hidden
    // rather than shown as dead buttons.
    const kindByTab = { all:null, movies:"movie", tvshows:"tvShow",
                        documentaries:"documentary", audiobooks:"audiobook",
                        books:"ebook" };
    const EMPTY_HIDABLE_TABS = ["movies", "tvshows", "documentaries", "audiobooks", "books"];

    function populatedLibraryTabs(catalog = items) {
      const populated = new Set(["all", "storage", "optimize"]);
      for (const item of catalog || []) {
        if (!item || item.isPlaceholder) continue;
        const tab = EMPTY_HIDABLE_TABS.find(candidate => kindByTab[candidate] === item.kind);
        if (tab) populated.add(tab);
      }
      return populated;
    }

    function tabIsHidden(tab) {
      if (typeof document === "undefined") return false;
      const button = document.querySelector(`#tabs button[data-tab="${tab}"]`);
      return !!button?.hidden;
    }

    // Empty media kinds stay configured and available in Settings, but do not
    // occupy the primary navigation by default. Turning the preference off
    // restores every tab so an empty kind can still be opened while it is
    // being configured.
    function applyEmptyLibraryTabs() {
      if (typeof document === "undefined") return false;
      const populated = populatedLibraryTabs();
      let activeTabReset = false;
      for (const button of document.querySelectorAll("#tabs button[data-tab]")) {
        const tab = button.dataset.tab;
        const hide = hideEmptyLibraries && EMPTY_HIDABLE_TABS.includes(tab) && !populated.has(tab);
        button.hidden = hide;
        if (hide) {
          button.setAttribute("aria-hidden", "true");
          button.tabIndex = -1;
          if (activeTab === tab) activeTabReset = true;
        } else {
          button.removeAttribute("aria-hidden");
          button.tabIndex = 0;
        }
      }
      if (activeTabReset) {
        activeTab = "all";
        openShow = null;
        openSeason = null;
        selectedListID = null;
        currentPage = 1;
        const back = $("#backRow");
        if (back) back.hidden = true;
        const seasons = $("#seasonList");
        if (seasons) seasons.innerHTML = "";
      }
      for (const button of document.querySelectorAll("#tabs button")) {
        button.classList.toggle("active", button.dataset.tab === activeTab);
      }
      return activeTabReset;
    }

    const FACETS = [
      { key: "genres",    label: "Genres" },
      { key: "authors",   label: "Authors" },
      { key: "narrators", label: "Narrators" },
      { key: "series",   label: "Series" },
      { key: "studios",   label: "Studios" },
    ];
    const FACET_LABEL = Object.fromEntries(FACETS.map(f => [f.key, f.label]));
    // Provider bookkeeping that must never surface as a genre.
    const PROVIDER_MARKERS = new Set(["wikipedia", "open-library", "audnexus", "plex", "tmdb", "imdb", "tvdb"]);

    // --- genre taxonomy ------------------------------------------------------
    // Genres arrive as hundreds of near-unique strings ("political science",
    // "political science--philosophy", "science politique--philosophie"), so the
    // Genres row leads with a short list of super-categories and only shows the
    // sub-genres of the one you open. Rules are ordered and the first match
    // wins, so narrow clusters are listed before broad ones. Nothing is thrown
    // away: a value matching no rule falls into "Unsorted" and stays clickable.
    const GENRE_CATEGORIES = [
      { key: "fiction", label: "Fiction & Literature", patterns: [
        /\bfiction/, /\bnovels?\b/, /\bliterature\b/, /\bliterary\b/, /\bpoetry\b/, /\bpoems?\b/,
        /\bdrama\b/, /\bfantasy\b/, /science fiction/, /\bhorror\b/, /\bthriller/, /\bsuspense/,
        /\bmystery\b/, /\bdetective/, /\bcrime\b/, /\bromance\b/, /\badventurous\b/, /\badventure/,
        /\bshort stories\b/, /\bjuvenile\b/, /young adult/, /children's (fiction|stories)/,
        /\bfairy tales\b/, /\bmytholog/, /\bgraphic novels?\b/, /\bcomics?\b/, /\bsatire\b/,
        /\bhumou?r\b/, /\bepic\b/, /\bwestern stories\b/, /\bnonfiction\b|\bnon-fiction\b/,
        /\bmurder\b/, /criticism and interpretation/, /\badaptations?\b/, /ficción juvenil/,
      ]},
      { key: "psychology", label: "Psychology & Self-Help", patterns: [
        /\bpsycholog/, /\bpsychoanaly/, /self-help/, /self-improvement/, /self-esteem/,
        /self-actualization/, /self-care/, /\bmotivat/, /\bbehaviou?r/, /\bemotions?\b/,
        /émotions/, /\bfeelings\b/, /\bsuccess\b/, /\bcoping\b/, /\bmindfulness/, /\bhabits?\b/,
        /neurodiversity/, /\btherapy\b/, /addict/, /substance abuse/, /\bsmoking/, /\bphobias?\b/,
        /\bconsciousness\b/, /\bguilt\b/, /\bshame\b/, /\bpleasure\b/, /brainwashing|brainwash/,
        /\bbrain\b/, /\battitude/, /\bpersonality/, /\bmental health/, /\bstress\b/, /\bhappiness/,
        /\bconscience/, /\baltruism/, /eudaimon|eudaemonics/, /\bfriendship/, /\bautis/,
        /\blistening\b/, /\bmorale\b/, /saddness|sadness/, /\bcharacter\b/, /\bmind\b/,
        /\bcompulsive/, /human behavio/, /\binfluence\b/, /\binterpersonal/,
      ]},
      { key: "philosophy", label: "Philosophy & Religion", patterns: [
        /\bphilosoph/, /\bethic/, /\bmoral/, /\breligion/, /\btheolog/, /\bchristian/, /\bcatholic/,
        /\bprotestant/, /\bbuddh/, /\bhindu/, /\bkrishna/, /\bbhagavad/, /\bbhakti/, /\byoga\b/,
        /\bmeditation/, /\bspiritual/, /\bmetaphysic/, /\bontology\b/, /\bepistemolog/,
        /existential/, /\bvirtues?\b/, /\bvertus\b/, /\bgod\b/, /\bprayer/, /\bcults?\b/,
        /\bwitchcraft/, /society of friends/, /\bquakers?\b/, /\bfaith\b/, /\bbelief\b/,
        /\bidealism/, /materialism/, /\bsublime\b/, /\babsolute\b/, /\bconduct of life\b/,
        /\bself\b/, /\bpostmodern/, /ethik|ethiek/, /\bglaube\b|\bgeloof\b|\bfoi\b/,
        /\bchristendom/, /filozofija|filosofie/, /\bdeterminism/, /\bstoic/, /\bmarxis/,
        /matérialisme|materialisme/,
      ]},
      { key: "history", label: "History & Biography", patterns: [
        /\bhistory\b/, /\bhistorical\b/, /\bbiograph/, /\bautobiograph/, /\bmemoir/, /\bdiaries\b/,
        /world war/, /\bancient\b/, /\bmedieval\b/, /\bantiquit/, /\bcivilization/, /\bcampaigns\b/,
        /\bpresidents\b/, /\bpioneers?\b/, /frontier and pioneer/, /concentration camps/, /\bgenocide/,
        /\bempire\b/, /\bmonarchy\b/, /\b20th century\b/, /\b19th century\b/, /\b18th century\b/,
        /\bnormandy/, /september 11/, /persian gulf war/, /\biraq war/, /\bkorea\b/, /cold war/,
        /\bunited states\b/, /\bdeath and burial\b/, /\bearly works\b/, /avant 1800/,
      ]},
      { key: "politics", label: "Politics, War & Society", patterns: [
        /\bpolitic/, /government/, /\bideolog/, /\bmarxis/, /\bcommunis/, /\bcapitalis/, /\bsocial/,
        /\bsociolog/, /\bculture/, /\bdemocracy/, /\banarchis/, /\bradicalis/, /terroris/,
        /\bpropaganda/, /\bpropoganda/, /public opinion/, /public relations/, /human rights/,
        /\bfeminism/, /\brace\b/, /\bcaste\b/, /\blaw\b/, /\blegal\b/, /\bviolence/, /\bwar\b/,
        /\bmilitary\b/, /\btanks?\b/, /\btank warfare/, /arms control/, /\bnuclear/, /\bbombers?\b/,
        /international relations/, /globali[sz]ation/, /\bimmigration/, /\bpoverty/, /\bjustice\b/,
        /\bpeace\b/, /nonviolence/, /political prisoners/, /\belections?\b/, /\bpolice\b/,
        /forced labor/, /\bslavery/, /\bcolon/, /separatis/, /nationalis/, /\bimperiali/, /état\b/,
        /\bfreedom\b/, /\bequality/, /\bcommunity\b/, /\bwomen\b/, /\bmen\b/, /\bamericans\b/,
        /\bcelebrit/, /\bjournalists?\b/, /\bscientists?\b/, /\bintellectual/, /\bmulticulturalism/,
        /foreign relations/, /\bluddism/,
        /\bpatients?\b/, /\bwidowers?\b/, /\bpersons?\b/, /violência|geweld/, /\bpublicity/,
        /\bstate\b/, /\bdružba|\bdruz ba/, /\bcommunication/,
      ]},
      { key: "business", label: "Business & Economics", patterns: [
        /\bbusiness/, /\beconom/, /\bfinance/, /\bfinancial/, /\binvest/, /\bstocks?\b/, /\bmarkets?\b/,
        /marketing/, /\baccounting/, /\bmanagement/, /\bentrepreneur/, /\bmoney\b/, /\bbanking/,
        /\btrade\b/, /\bvaluation/, /corporations/, /\bprices\b/, /\bretirement/, /leadership/,
        /\bcareer/, /\bsales\b/, /\bportfolio/, /derivative securities/, /interest rates/,
        /\bcash flow/, /\bindustry/, /\bbusinesspeople/, /\bbillionaires/, /\bcommerce/,
        /aktienanalyse/, /börsenhandel/, /finanzbuchhaltung/, /investitionstheorie/,
        /gestion de portefeuille/, /business communication/, /\bbudget/, /\bnegotiat/,
        /\bforecasting/, /\bspeculation/, /financial statements/, /long-term planning/,
      ]},
      { key: "science", label: "Science & Technology", patterns: [
        /\bscience/, /\bscientific/, /\bphysics/, /\bchemistry/, /\bbiolog/, /\bmathematic/,
        /\bstatistics/, /\bgeometry/, /\bmeetkunde/, /\btechnolog/, /\bcomput/, /\bsoftware/,
        /\bprogramming/, /\bdata\b/, /machine learning/, /artificial intelligence/, /\bneural/,
        /\bdeep learning/, /\balgorithm/, /\bnetworks?\b/, /\bsecurity/, /\bdatabase/, /\bjava\b/,
        /\bpython\b/, /\bhtml\b/, /\bunix\b/, /\blinux\b/, /\bmysql/, /\bphp\b/, /operating systems/,
        /\bservlets/, /virtual computer/, /vmware/, /\bcyber/, /rootkits?/, /\bhackers?\b/,
        /\bengineer/, /\bmedicine\b/, /\bmedical\b/, /\bastronom/, /\bevolution\b/, /\becolog/,
        /\benvironment/, /\bnature\b/, /\banimals?\b|\banimales\b/, /\bfoxes\b/, /\bzorros\b/,
        /\bgenetic/, /\bquantum/, /\benergy\b/, /\bcausation/, /\bmatter\b/, /fourth dimension/,
        /extraterrestrial/, /uranium/, /rocket engines/, /propellants/, /computer animation/,
        /\bcs\.[a-z_]/, /\bcom\d{6}/, /\bweb\b/, /internet/, /\bcyberspace/, /\btechnical/, /\bbash\b/,
      ]},
      { key: "lifestyle", label: "Lifestyle, Health & Home", patterns: [
        /\bcooking/, /\bcookery/, /\brecipes/, /\bfood\b/, /\bdiet/, /weight loss/, /\bnutrition/,
        /\bfitness/, /\bexercise/, /physical fitness/, /\bhealth/, /\bhygiene/, /\bgardening/,
        /home economics/, /\bfashion/, /\btravel/, /description and travel/, /\bsports?\b/,
        /\btennis/, /bodybuilding/, /\bwellness/, /\bfamily\b/, /relationships?/, /\bmarriage/,
        /\bparenting/, /man-woman relationships/, /\bhousehold/, /\bentertainment/, /\bhobbies/,
        /\bcrafts/, /\bsurvival\b/, /wilderness/, /\bshoulder/, /wounds and injuries/,
        /rehabilitation/, /occupational therapy/, /\bsleep\b/, /\betiquette/, /\blifestyle/,
        /\btransportation/, /\bhome\b/, /kinesiolog/,
      ]},
      { key: "reference", label: "Reference & Education", patterns: [
        /\bdictionary/, /\bencyclopedia/, /reference/, /\bexaminations/, /study guides/,
        /handbooks/, /\bmanuals/, /\bteaching/, /\beducation/, /\bschools?\b/, /\bcurriculum/,
        /\bstudents?\b/, /study and teaching/, /\blanguage/, /\blinguistic/, /\bwriting/,
        /\bpublishing/, /\bbibliograph/, /\bperiodicals/, /\bsanskrit/, /\benglish\b/, /\bgrammar/,
        /\bvocabulary/, /large type books/, /\bcatalogs?\b/, /\bdirectories/, /\bresearch\b/,
        /\blibrary/, /\bbooksellers/, /collections & anthologies/, /questions, etc/,
        /\bmethods\b/, /pictorial works/, /\bauthors\b/, /\beditors\b/,
      ]},
      { key: "people", label: "People & Characters", patterns: [] },
    ];
    const GENRE_UNSORTED = { key: "unsorted", label: "Unsorted", patterns: [] };
    // Values whose obvious keyword points at the wrong super-category.
    const GENRE_OVERRIDES = new Map([
      ["computer crimes", "science"], ["computer crime", "science"],
      ["true crime", "fiction"], ["war on terrorism, 2001-2009", "politics"],
      ["the future", "politics"], ["the state", "politics"],
      ["database management", "science"], ["catch-22 (heller, joseph)", "fiction"],
    ]);

    // A person's name: "surname, forename, dates" or two to four name-like
    // words. The looser shape is judged only after every genre rule has passed
    // and only when no subject word appears, so "judith butler" is a person but
    // "intellectual life" is not.
    const PERSON_DATED = /^\p{L}[^,]*,\s*\p{L}[^,]*,\s*\d{3,4}/u;
    const PERSON_NAME = /^\p{L}[\p{L}.'-]*(?:\s+\p{L}[\p{L}.'-]*){1,4}$/u;
    const NOT_A_PERSON = /\b(life|times?|reviewed|bestseller|criticism|interpretation|determinism|society|social|politic|philosoph|science|literature|theory|studies|study|analysis|relations|aspects|conditions|development|management|design|construction|planning|policy|change|control|behaviou?r|health|nutrition|education|schools?|staff|picks|library|data|computers?|finance|business|markets?|investment|law|ethic|state|states|future|people|persons?|women|men|children|workers|prisoners|editors|authors?|readers|immigrants|americans|journalists?|scientists?|patients?|celebrit|intellectuals?|motion|pictures?|war|history|culture|religion|art|music|econom|psychology|technolog|systems?|models?|methods?|research|media|press|books?|publishing|language|writing|practices?|services?|industry|government|power|gender|race|class|rights|movements?|groups?|natural|human|world|public|private|modern)\b/i;

    // Which super-category a genre value belongs to.
    function genreCategoryFor(label) {
      const value = String(label).toLowerCase();
      const override = GENRE_OVERRIDES.get(value);
      if (override) return override;
      if (/fictitious character/.test(value)) return "people";
      for (const category of GENRE_CATEGORIES) {
        if (category.key === "people") continue;   // judged last, on shape alone
        if (category.patterns.some(re => re.test(value))) return category.key;
      }
      if (PERSON_DATED.test(value)) return "people";
      if (PERSON_NAME.test(value) && !NOT_A_PERSON.test(value)) return "people";
      return GENRE_UNSORTED.key;
    }

    // Titles per super-category, counting each item once however many of its
    // sub-genres share the category.
    function genreCategoryCounts() {
      const counts = new Map();
      for (const item of facetUniverse()) {
        const seen = new Set();
        for (const raw of facetValues(item, "genres")) {
          const label = String(raw).trim();
          if (label) seen.add(genreCategoryFor(label));
        }
        for (const key of seen) counts.set(key, (counts.get(key) || 0) + 1);
      }
      return counts;
    }

    // Values shown before the list is collapsed behind a "+N more" toggle.
    // Book subjects alone can reach several hundred entries, so the chip row
    // wraps onto multiple lines but still needs a lid on it.
    const CHIP_LIMIT = 18;

    let activeFacet = "genres";
    let selectedFacetValues = new Set();
    let facetQuery = "";
    let facetModesSignature = "";
    let chipsExpanded = false;
    // Open super-category in the Genres facet, or null for the category list.
    let genreCategory = null;

    function facetValues(item, key) {
      switch (key) {
        case "genres": {
          const genres = Array.isArray(item.genres) ? item.genres.filter(Boolean) : [];
          if (genres.length) return genres;
          // Legacy rows kept genres inside tags, mixed with provider markers
          // and people names; strip those rather than label them as genres.
          const people = new Set([item.author, item.narrator]
            .filter(Boolean).map(v => String(v).toLowerCase()));
          return (Array.isArray(item.tags) ? item.tags : []).filter(tag => {
            const label = String(tag || "").trim();
            if (!label) return false;
            const lower = label.toLowerCase();
            return !PROVIDER_MARKERS.has(lower) && !people.has(lower);
          });
        }
        case "authors":   return item.author ? [item.author] : [];
        case "narrators": return item.narrator ? [item.narrator] : [];
        case "series":   return item.series ? [item.series] : [];
        case "studios":   return item.studio ? [item.studio] : [];
        default:          return [];
      }
    }

    // Only the current tab's items contribute facet values and counts.
    function facetUniverse() {
      const kindSel = kindByTab[activeTab] ?? null;
      return items.filter(i => !i.isPlaceholder && (!kindSel || i.kind === kindSel));
    }

    // facet key -> Map(valueLower -> { label, count })
    function facetIndex() {
      const index = new Map();
      for (const facet of FACETS) {
        const values = new Map();
        for (const item of facetUniverse()) {
          const seen = new Set();
          for (const raw of facetValues(item, facet.key)) {
            const label = String(raw).trim();
            if (!label) continue;
            const key = label.toLowerCase();
            if (seen.has(key)) continue;   // count each item once per value
            seen.add(key);
            const entry = values.get(key) || { label, count: 0 };
            entry.count++;
            values.set(key, entry);
          }
        }
        if (values.size) index.set(facet.key, values);
      }
      return index;
    }

    function renderFacets() {
      const host = $("#metadataChips"), modesHost = $("#filterModes"), row = $("#filterRow");
      if (!host || !modesHost || !row) return;

      const index = facetIndex();
      if (!index.has(activeFacet)) activeFacet = index.keys().next().value || "";

      row.hidden = index.size === 0;
      if (!index.size) {
        host.innerHTML = "";
        host.classList.remove("expanded");
        modesHost.innerHTML = "";
        if ($("#chipsFooter")) { $("#chipsFooter").innerHTML = ""; $("#chipsFooter").hidden = true; }
        facetModesSignature = "";
        return;
      }

      // Rebuild the mode buttons only when their set or active state changes,
      // so chip clicks don't steal focus from the mode row.
      const signature = [...index.keys()].join(",") + "|" + activeFacet;
      if (signature !== facetModesSignature) {
        modesHost.innerHTML = [...index.keys()].map(key =>
          `<button type="button" data-filter-mode="${key}" class="${key === activeFacet ? "active" : ""}" aria-pressed="${key === activeFacet}">${FACET_LABEL[key] || key}</button>`
        ).join("");
        facetModesSignature = signature;
      }

      const values = index.get(activeFacet);
      for (const key of [...selectedFacetValues]) {
        if (!values.has(key)) selectedFacetValues.delete(key);
      }
      const label = (FACET_LABEL[activeFacet] || "values").toLowerCase();
      const search = $("#facetSearch"), clear = $("#facetClear");
      if (search) {
        search.hidden = values.size <= 8;
        search.placeholder = `Find ${label}…`;
        if (!search.hidden && search.value !== facetQuery) search.value = facetQuery;
      }
      if (clear) {
        const n = selectedFacetValues.size;
        clear.hidden = n === 0;
        clear.textContent = n > 1 ? `Clear ${n} filters` : "Clear filter";
      }

      let entries = [...values.entries()];
      if (facetQuery) entries = entries.filter(([, entry]) => entry.label.toLowerCase().includes(facetQuery));
      // Most-used values first: with hundreds of book subjects, the useful
      // ones should not be hidden at the end of a long scroll.
      entries.sort((a, b) => b[1].count - a[1].count || a[1].label.localeCompare(b[1].label));

      // The Genres facet is grouped: level 1 offers the super-categories and
      // level 2 the sub-genres of the one that was opened. A search always
      // spans every category, so a match cannot hide behind a closed one.
      const grouped = activeFacet === "genres" && !facetQuery;
      let listed = entries;
      // Selections the current view would not otherwise list, so an active
      // filter is always visible from wherever it was set.
      let pinned = [];
      let lead = "";

      if (grouped && !genreCategory) {
        const counts = genreCategoryCounts();
        // Only categories this tab actually has values for become buttons.
        const present = new Set(entries.map(([, entry]) => genreCategoryFor(entry.label)));
        const categories = [...GENRE_CATEGORIES, GENRE_UNSORTED].filter(c => present.has(c.key));
        pinned = entries.filter(([key]) => selectedFacetValues.has(key));
        const chips = [
          allChipHTML(),
          ...pinned.map(([key, entry]) => valueChipHTML(key, entry)),
          ...categories.map(category => {
            const count = counts.get(category.key) || 0;
            const active = [...selectedFacetValues].some(key =>
              (values.get(key) && genreCategoryFor(values.get(key).label)) === category.key);
            return `<button type="button" class="chip category${active ? " has-selection" : ""}" data-genre-category="${category.key}" title="Show ${escapeHTML(category.label)} sub-genres">${escapeHTML(category.label)}<span class="chip-count">${count}</span></button>`;
          }),
        ];
        host.classList.remove("expanded");
        host.innerHTML = chips.join("");
        setChipsFooter("");
        return;
      }
      if (grouped) {
        listed = entries.filter(([, entry]) => genreCategoryFor(entry.label) === genreCategory);
        pinned = entries.filter(([key, entry]) =>
          selectedFacetValues.has(key) && genreCategoryFor(entry.label) !== genreCategory);
        const category = [...GENRE_CATEGORIES, GENRE_UNSORTED].find(c => c.key === genreCategory);
        lead = `<button type="button" class="chip back" data-genre-back>‹ ${escapeHTML(category ? category.label : "All genres")}</button>`;
      }

      // Collapse the tail behind a toggle, but never hide a value the user
      // has already selected: a filter you cannot see is a filter you cannot
      // switch off.
      const overflowing = !chipsExpanded && !facetQuery && listed.length > CHIP_LIMIT;
      const shown = overflowing
        ? listed.filter(([key], i) => i < CHIP_LIMIT || selectedFacetValues.has(key))
        : listed;
      const hidden = listed.length - shown.length;

      const chips = [lead, allChipHTML()];
      for (const [key, entry] of [...pinned, ...shown]) chips.push(valueChipHTML(key, entry));
      if (listed.length === 0 && pinned.length === 0) {
        chips.push(`<span class="chips-empty" role="status">No ${label} match “${escapeHTML(facetQuery)}”</span>`);
      }
      const bounded = chipsExpanded && !facetQuery && listed.length > CHIP_LIMIT;
      host.classList.toggle("expanded", bounded);
      host.innerHTML = chips.join("");

      setChipsFooter(hidden > 0
        ? `<button type="button" class="chips-more" data-chips-toggle="more" aria-expanded="false">+${hidden} more</button>`
        : bounded
          ? `<button type="button" class="chips-more" data-chips-toggle="less" aria-expanded="true">Show fewer</button>`
          : "");
    }

    function allChipHTML() {
      const all = selectedFacetValues.size === 0;
      return `<button type="button" class="chip ${all ? "selected" : ""}" data-facet-value="" aria-pressed="${all}">All</button>`;
    }

    function valueChipHTML(key, entry) {
      const selected = selectedFacetValues.has(key);
      return `<button type="button" class="chip ${selected ? "selected" : ""}" data-facet-value="${escapeHTML(entry.label)}" aria-pressed="${selected}" title="${escapeHTML(entry.label)} — ${entry.count} title${entry.count === 1 ? "" : "s"}">${escapeHTML(entry.label)}<span class="chip-count">${entry.count}</span></button>`;
    }

    function setChipsFooter(html) {
      const footer = $("#chipsFooter");
      if (!footer) return;
      footer.innerHTML = html;
      footer.hidden = !html;
    }

    // Delegated handlers: the chip/mode containers persist across re-renders.
    on("#filterModes", "click", event => {
      const button = event.target.closest("[data-filter-mode]");
      if (!button) return;
      activeFacet = button.dataset.filterMode;
      selectedFacetValues.clear();
      facetQuery = "";
      chipsExpanded = false;
      genreCategory = null;
      const search = $("#facetSearch");
      if (search) search.value = "";
      renderFacets();
      render();
      // Focus follows the active mode button after the row is rebuilt.
      $("#filterModes").querySelector("[aria-pressed='true']")?.focus();
    });
    on("#metadataChips", "click", event => {
      const category = event.target.closest("[data-genre-category]");
      if (category) {
        genreCategory = category.dataset.genreCategory;
        chipsExpanded = false;
        renderFacets();
        // The row is rebuilt; land on the back chip that now leads it.
        $("#metadataChips").querySelector("[data-genre-back]")?.focus();
        return;
      }
      if (event.target.closest("[data-genre-back]")) {
        const previous = genreCategory;
        genreCategory = null;
        chipsExpanded = false;
        renderFacets();
        $("#metadataChips").querySelector(`[data-genre-category="${CSS.escape(previous)}"]`)?.focus();
        return;
      }
      const button = event.target.closest("[data-facet-value]");
      if (!button) return;
      const value = button.dataset.facetValue.toLowerCase();
      if (!value) selectedFacetValues.clear();
      else if (selectedFacetValues.has(value)) selectedFacetValues.delete(value);
      else selectedFacetValues.add(value);
      renderFacets();
      render();
      // The chip DOM is rebuilt, so move focus back onto the same value.
      const target = host.querySelector(`[data-facet-value="${CSS.escape(value)}"]`)
                  || host.querySelector('[data-facet-value=""]');
      target?.focus();
    });
    on("#chipsFooter", "click", event => {
      const toggle = event.target.closest("[data-chips-toggle]");
      if (!toggle) return;
      chipsExpanded = toggle.dataset.chipsToggle === "more";
      renderFacets();
      // The footer is rebuilt, so focus the replacement toggle.
      $("#chipsFooter").querySelector("[data-chips-toggle]")?.focus();
    });
    on("#facetSearch", "input", event => {
      facetQuery = event.target.value.trim().toLowerCase();
      renderFacets();
    });
    on("#facetClear", "click", () => {
      selectedFacetValues.clear();
      renderFacets();
      render();
    });

    on("#coverFilter", "change", () => {
      currentPage = 1;
      render();
    });

    function kindLabel(v) {
      return ({ movie:"Movie", tvShow:"TV Show", documentary:"Documentary",
                audiobook:"Audiobook", ebook:"Book", all:"All" })[v] || v;
    }
    function progressFor(id) { return progressByID.get(id) ?? null; }
    function isWatched(p, item) {
      if (!p || p.seconds <= 0) return false;
      const dur = p.duration || item.durationSeconds || 0;
      return dur > 0 && p.seconds / dur >= 0.96;
    }
    function inProgress(p, item) {
      if (!p || p.seconds <= 5) return false;
      return !isWatched(p, item);
    }
    function runtimeLabel(item) {
      const d = item.durationSeconds;
      if (!d || d <= 0) return "";
      return formatTime(d);
    }

    // Collapse same-title copies into one card per title, best copy first.
    // Ranking prefers probed runtime > resolution > format browser-playability.
    const FORMAT_RANK = { mp4:3, m4v:3, mov:3, webm:3, mkv:2, avi:1 };
    function titleHeight(i) {
      // Fall back to a resolution hint embedded in the title ("720p", "1080P").
      const m = (i.title || "").match(/(\d{3,4})\s*p\b/i);
      return m ? parseInt(m[1], 10) : 0;
    }
    function copyHeight(i) {
      return i.probedHeight || titleHeight(i);
    }
    function rankCopy(i) {
      // Resolution dominates: a higher-definition copy is the default even
      // if the low-res one has probe data and the HD one doesn't.
      return copyHeight(i) * 1e8
           + ((i.durationSeconds || 0) > 0 ? 1e6 : 0)
           + (FORMAT_RANK[(i.format||"").toLowerCase()] || 0) * 1e2;
    }
    let dedupEnabled = true;
    function visibleItems() {
      const term = $("#q").value.trim().toLowerCase();
      const watchSel = $("#watched").value;
      let list = items.filter(i => !i.isPlaceholder);
      // Tabs are the kind filter; movies and documentaries remain distinct
      // catalogs even when they share video playback behavior.
      const kindSel = kindByTab[activeTab] ?? null;
      if (kindSel) list = list.filter(i => i.kind === kindSel);
      const coverFilter = $("#coverFilter")?.value || "all";
      if (coverFilter !== "all") {
        list = list.filter(i => i.kind === "audiobook" && (
          coverFilter === "has-cover" ? i.coverAvailable === true
          : coverFilter === "missing-cover" ? i.coverAvailable !== true
          : i.coverAvailable === true && i.coverEmbedded !== true
        ));
      }
      if (selectedFacetValues.size) list = list.filter(i => facetValues(i, activeFacet).some(v => selectedFacetValues.has(String(v).toLowerCase())));
      if (term) {
        list = list.filter(i =>
          [i.title, i.subtitle, i.showTitle, i.summary]
            .some(f => f && String(f).toLowerCase().includes(term)));
      }
      switch ($("#sort").value) {
        case "year":     list.sort((a,b)=>(b.year||0)-(a.year||0)||a.title.localeCompare(b.title)); break;
        case "duration": list.sort((a,b)=>(b.durationSeconds||0)-(a.durationSeconds||0)); break;
        case "series":   list.sort((a,b)=>(a.series || "").localeCompare(b.series || "") || ((a.seriesNumber || 1e9) - (b.seriesNumber || 1e9)) || a.title.localeCompare(b.title)); break;
        case "recent":   list.sort((a,b)=>{
                           const ua = progressByID.get(a.id)?.updatedAt || "";
                           const ub = progressByID.get(b.id)?.updatedAt || "";
                           return ub.localeCompare(ua) || a.title.localeCompare(b.title);
                         }); break;
        default:         list.sort((a,b)=>a.title.localeCompare(b.title));
      }
      if (dedupEnabled) {
        // Keep the best copy of each (kind,title); default to the highest
        // definition (rankCopy), but a copy with watch progress wins so the
        // user resumes where they left off. Keys are the MERGED group keys
        // (exact + fuzzy roots) so variants like "Good Will Hunting V2"
        // fold into "Good Will Hunting" here too. Groups may have been
        // rebuilt for a different render scope; membership maps always hold.
        const byKey = new Map();
        for (const i of list) {
          const key = itemGroupKey.get(i.id) ?? copyKey(i);
          const prev = byKey.get(key);
          if (!prev) { byKey.set(key, i); continue; }
          const prevP = progressFor(prev.id), curP = progressFor(i.id);
          const prevActive = inProgress(prevP, prev) || isWatched(prevP, prev);
          const curActive = inProgress(curP, i) || isWatched(curP, i);
          const better = curActive !== prevActive ? curActive
                       : rankCopy(i) > rankCopy(prev);
          if (better) byKey.set(key, i);
        }
        list = Array.from(byKey.values());
      }
      if (watchSel !== "all") {
        list = list.filter(i => watchSel === "watched" ? isWatched(progressFor(i.id), i)
                                                       : !isWatched(progressFor(i.id), i));
      }
      return list;
    }

    // All copies of each collapsed title, for the edition picker and the
    // per-card copy badge. Groups are rebuilt once per data/progress change
    // (rebuildCopyGroups) instead of rescanning the full catalog per card.
    // Keys normalize punctuation/case and strip embedded release tags so
    // "Tucker: The Man..." / "Tucker The Man..." collapse, as do titles
    // carrying " 1080p BluRay x265" style suffixes.
    let copyGroups = new Map();
    let itemGroupKey = new Map();   // item.id -> merged group key (incl. fuzzy roots)
    const COPY_GROUP_CACHE_KEY = "sonder.copy-groups.v1";
    function foldDiacritics(s) {
      return s.normalize("NFD").replace(/[\u0300-\u036f]/g, "");
    }
    function movieSplitPart(i) {
      if (i.splitPart) return String(i.splitPart).toLowerCase();
      if (i.kind !== "movie" && i.kind !== "documentary") return "";
      const title = String(i.title || "").toLowerCase();
      let m = title.match(/\b(?:part|pt)\s*([ivxlcdm]+|\d+)\b/i);
      if (m) return "part" + m[1].toLowerCase();
      m = title.match(/(?:\b|[^a-z0-9])\(?(\d+)of\d+\)?\s*$/i);
      if (m) return "part" + m[1];
      m = title.match(/(?:^|[\s._-])(cd|disc|disk|dvd)(\d+)\s*$/i);
      if (m) return m[1].toLowerCase() + m[2];
      return "";
    }
    function copyKey(i) {
      // Episode titles recur across shows and seasons (especially "Pilot").
      if (i.kind === "tvShow" && i.showTitle && i.seasonNumber != null && i.episodeNumber != null) {
        const parsedShow = foldDiacritics(i.showTitle).toLowerCase().replace(/\(\d{4}\)/g, "").replace(/[^a-z0-9]/g, "");
        return [i.kind, i.showGroupID || "", parsedShow, i.seasonNumber, i.episodeNumber, i.splitPart || ""].join("\u0000");
      }
      let t = foldDiacritics(i.title || "").toLowerCase();
      t = t.replace(/\b\d{3,4}\s*p\b/gi, "")           // resolution hints
           .replace(/[\[\(\{][^\]\)\}]*[\]\)\}]/g, "")  // any (...) [...] {...} tag group
           .replace(/(?:bluray|webrip|web[- ]?dl|hdtv|hdrip|dvdrip|bdrip|brrip|remux|x265|x264|xvid|hevc|h264|divx|mpeg[- ]?4|10bit|8bit|aac|ddp5|yify|rarbg|mkvcage|s4filmes|proper|repack|unrated)\b/gi, "")
           .replace(/\b(dual|multi|subs?|ws)\b/gi, "")
           .replace(/\bv\d+\b/gi, "")                   // V2 / v3 re-encode tags
           .replace(/[^a-z0-9]+/g, "");
      return [i.kind, t, i.year || 0, movieSplitPart(i), i.kind === "tvShow" ? (i.showGroupID || i.showTitle || i.id) : "", i.kind === "tvShow" ? (i.seasonNumber ?? "unknown") : ""].join("\u0000");
    }
    // Near-duplicate folding: typos ("007 Jame" vs "007 James"), junk tails
    // ("-1", " a", "-cd1", "800MB"), diacritics (Nausicaa/Nausicaä), and
    // release-tag variants get merged into the same card. Guards keep real
    // sequels/plurals separate: different trailing numbers ("Predator" vs
    // "Predators", "Sherlock Holmes" vs "Holmes 2") or disjoint years never
    // merge. Ratios calibrated on the live catalog (folds verified correct).
    function diceRatio(a, b) {
      if (a === b) return 1;
      if (a.length < 4 || b.length < 4) return 0;
      const A = new Set(), B = new Set();
      for (let i = 0; i < a.length - 1; i++) A.add(a.slice(i, i + 2));
      for (let i = 0; i < b.length - 1; i++) B.add(b.slice(i, i + 2));
      let inter = 0;
      for (const x of A) if (B.has(x)) inter++;
      return 2 * inter / (A.size + B.size);
    }
    const FUZZ_MIN = 0.85;
    function externalMediaKey(i) {
      if (i.kind !== "movie" && i.kind !== "documentary") return "";
      const source = String(i.metadataIDSource || "").trim().toLowerCase();
      const id = String(i.metadataID || "").trim().toLowerCase();
      if (!source || !id) return "";
      // Keep split/disc files separate even when the provider ID is shared.
      return [source, id, movieSplitPart(i)].join("\u0000");
    }
    function copyTitle(k) {
      return k.split("\u0000")[0];
    }
    function numTail(k) {
      const m = copyTitle(k).match(/(\d+|[ivxl]{1,5})$/);
      return m ? m[1] : null;
    }
    function isPluralPair(a, b) {
      // Word-boundary plural: "predator"/"predators". Only when the shorter
      // ends at a whole-token boundary of the longer and the extra letters
      // are just an s/es tail.
      a = copyTitle(a);
      b = copyTitle(b);
      const short = a.length <= b.length ? a : b;
      const long = a.length <= b.length ? b : a;
      return long.startsWith(short) && /^(?:es|s)$/.test(long.slice(short.length));
    }
    // Generic bonus-feature titles: identical across films ("Trailer" under
    // every Featurettes/ dir) but different content. Keys are post-copyKey
    // normalized titles; matching items always get solo cards.
    const GENERIC_EXTRAS = new Set([
      "trailer", "theatricaltrailer", "teaser",
      "deletedscenes", "behindthescenes", "makingof",
      "featurette", "bloopers", "outtakes", "gagreeel",
      "interview", "interviews", "commentary",
    ]);
    const GENERIC_EXTRA_PREFIXES = ["storyboardcomparison"];
    function isGenericExtra(k) {
      const title = k.split("\u0000")[1] || "";
      return GENERIC_EXTRAS.has(title) || GENERIC_EXTRA_PREFIXES.some(prefix => title.startsWith(prefix));
    }
    function rebuildCopyGroups() {
      listIndexGeneration++;
      copyGroups = new Map();
      const extrasGroups = new Map();  // generic bonus-feature titles: never merged
      const yearsByKey = new Map();
      for (const i of items) {
        if (i.isPlaceholder) continue;
        const k = copyKey(i);
        if (i.year) {
          let years = yearsByKey.get(k);
          if (!years) { years = new Set(); yearsByKey.set(k, years); }
          years.add(i.year);
        }
        if (isGenericExtra(k)) {
          // "Trailer" / "Deleted Scenes" from different films share a key but
          // are different content — one card per item.
          const solo = k + "\u0000" + i.id;
          if (!copyGroups.has(solo)) copyGroups.set(solo, []);
          copyGroups.get(solo).push(i);
          extrasGroups.set(solo, true);
          continue;
        }
        if (!copyGroups.has(k)) copyGroups.set(k, []);
        copyGroups.get(k).push(i);
      }
      // Union-find over near-identical keys (per kind), only for kinds with
      // meaningful dupes (movies/documentaries). TV uses exact keys only —
      // episode codes make fuzzy keys unreliable.
      // A packed-number episode with an explicit duplicate filename suffix
      // joins its unsuffixed sibling only when that sibling actually exists
      // in the same show/season. All files remain in the version picker.
      for (const [key, group] of [...copyGroups]) {
        const sample = group[0];
        if (sample.kind !== "tvShow" || sample.episodeNumber != null || !/^\d{3,4}\s*-/.test(sample.title || "")) continue;
        const baseTitle = sample.title.replace(/-\d+$/, "");
        if (baseTitle === sample.title) continue;
        const baseKey = copyKey({...sample, title:baseTitle});
        if (baseKey !== key && copyGroups.has(baseKey)) {
          copyGroups.get(baseKey).push(...group);
          copyGroups.delete(key);
        }
      }
      const kinds = new Set();
      for (const k of copyGroups.keys()) kinds.add(k.split("\u0000")[0]);
      const parent = new Map();
      const find = (k) => {
        let r = k;
        while (parent.get(r) !== r) r = parent.get(r);
        parent.set(k, r);
        return r;
      };
      const emptyYears = new Set();
      const yearCache = (kind, key) => yearsByKey.get(kind + "\u0000" + key) || emptyYears;
      for (const kind of kinds) {
        if (kind !== "movie" && kind !== "documentary") continue;
        const keys = [...copyGroups.keys()].filter(k => k.startsWith(kind + "\u0000"))
          .map(k => k.slice(kind.length + 1))
          .filter(t => !extrasGroups.has(kind + "\u0000" + t))
          .sort();
        for (const k of keys) parent.set(k, k);
        // A provider ID is authoritative for movies/documentaries. This also
        // joins a correctly tagged copy to a differently named copy of the
        // same title, while preserving separate split/disc parts.
        const externalRoots = new Map();
        for (const k of keys) {
          const group = copyGroups.get(kind + "\u0000" + k);
          const external = externalMediaKey(group?.[0]);
          if (!external) continue;
          const first = externalRoots.get(external);
          if (first && first !== k) {
            const ra = find(first), rb = find(k);
            if (ra !== rb) parent.set(rb, ra);
          } else {
            externalRoots.set(external, k);
          }
        }
        for (let i = 0; i < keys.length; i++) {
          for (let j = i + 1; j < Math.min(i + 6, keys.length); j++) {
            const a = keys[i], b = keys[j];
            if (Math.abs(a.length - b.length) > 3) continue;
            if (diceRatio(a, b) < FUZZ_MIN) continue;
            if (isPluralPair(a, b)) continue;              // Predator/Predators
            const ta = numTail(a), tb = numTail(b);
            if ((ta && !tb) || (tb && !ta)) continue;      // sequel guard
            if (ta && tb && ta !== tb) continue;
            const ya = yearCache(kind, a), yb = yearCache(kind, b);
            if (ya.size && yb.size && ![...ya].some(y => yb.has(y))) continue;
            // Identity suffixes (year/part) must agree even for fuzzy titles.
            if (a.slice(a.indexOf("\u0000")) !== b.slice(b.indexOf("\u0000"))) continue;
            const ra = find(a), rb = find(b);
            if (ra === rb) continue;
            const root = a.length <= b.length ? ra : rb;
            parent.set(ra, root); parent.set(rb, root);
          }
        }
        // Re-point every group key to its merged root bucket.
        // Solo extras cards bypass the union entirely. Keys of other kinds
        // are identity-mapped: find() only knows this kind's union, and
        // letting other kinds fall through would collapse each of them into
        // a single "<kind> undefined" bucket.
        itemGroupKey = new Map();
        for (const k of [...copyGroups.keys()]) {
          if (extrasGroups.has(k)) {
            for (const it of copyGroups.get(k)) itemGroupKey.set(it.id, k);
            continue;
          }
          const kindx = k.split("\u0000")[0], t = k.slice(kindx.length + 1);
          if (kindx !== kind) {
            for (const it of copyGroups.get(k)) itemGroupKey.set(it.id, k);
            continue;
          }
          const root = find(t);
          const rk = kindx + "\u0000" + root;
          if (root !== t) {
            if (!copyGroups.has(rk)) copyGroups.set(rk, []);
            copyGroups.get(rk).push(...copyGroups.get(k));
            copyGroups.delete(k);
          }
          for (const it of copyGroups.get(rk) || []) itemGroupKey.set(it.id, rk);
        }
      }
      sortCopyGroups();
    }

    function sortCopyGroups() {
      for (const group of copyGroups.values()) {
        group.sort((a,b) => {
          const pa = progressFor(a.id), pb = progressFor(b.id);
          const aa = inProgress(pa,a)||isWatched(pa,a), ab = inProgress(pb,b)||isWatched(pb,b);
          if (aa !== ab) return aa ? -1 : 1;
          return rankCopy(b) - rankCopy(a);
        });
      }
      // Build membership once after every kind has been merged. Reinitializing
      // it inside each kind's loop loses previous movie/documentary merges.
      itemGroupKey = new Map();
      for (const [key, group] of copyGroups) {
        for (const item of group) itemGroupKey.set(item.id, key);
      }
    }

    // Fuzzy copy matching is intentionally thorough, but its result only
    // changes when the catalog generation changes. Cache the compact
    // itemID -> merged-group map so a normal reload can rebuild the Maps in a
    // linear pass instead of repeating all fuzzy comparisons.
    function restoreCopyGroups(etag) {
      if (!etag || typeof localStorage === "undefined") return false;
      try {
        const cached = JSON.parse(localStorage.getItem(COPY_GROUP_CACHE_KEY) || "null");
        if (!cached || cached.etag !== etag || !Array.isArray(cached.entries)) return false;
        const expected = items.reduce((count, item) => count + (item.isPlaceholder ? 0 : 1), 0);
        const membership = new Map(cached.entries);
        if (membership.size !== expected) return false;

        const nextGroups = new Map();
        for (const item of items) {
          if (item.isPlaceholder) continue;
          const groupKey = membership.get(item.id);
          if (!groupKey) return false;
          if (!nextGroups.has(groupKey)) nextGroups.set(groupKey, []);
          nextGroups.get(groupKey).push(item);
        }
        itemGroupKey = membership;
        copyGroups = nextGroups;
        sortCopyGroups();
        return true;
      } catch (_) {
        return false;
      }
    }

    function cacheCopyGroups(etag) {
      if (!etag || typeof localStorage === "undefined") return;
      const save = () => {
        try {
          localStorage.setItem(COPY_GROUP_CACHE_KEY, JSON.stringify({
            etag,
            entries: [...itemGroupKey.entries()],
          }));
        } catch (_) {
          // Storage can be disabled or full; the uncached path remains valid.
        }
      };
      if (typeof requestIdleCallback === "function") requestIdleCallback(save, { timeout: 2000 });
      else setTimeout(save, 0);
    }
    function copiesOf(item) {
      const gk = itemGroupKey.get(item.id);
      if (gk) {
        const g = copyGroups.get(gk);
        if (g && g.length) return g;
      }
      return copyGroups.get(copyKey(item)) ?? [item];
    }

    function mainPageEntries(list) {
      const visibleIDs = new Set(list.map(i => i.id));
      const shows = buildShowGroups().filter(show => [...show.seasons.values()].some(eps =>
        eps.some(ep => copiesOf(ep).some(copy => visibleIDs.has(copy.id)))));
      return [...list.filter(i => i.kind !== "tvShow"),
        ...shows.map(show => ({ title: show.name, show }))]
        .sort((a, b) => a.title.localeCompare(b.title));
    }

    function cardHTML(item, opts = {}) {
      const p = progressFor(item.id);
      const watched = isWatched(p, item);
      const pct = (() => {
        if (!p || p.seconds <= 0) return 0;
        const dur = p.duration || item.durationSeconds || 0;
        return dur > 0 ? Math.min(100, p.seconds / dur * 100) : 0;
      })();
      const poster = item.posterURL
        ? `<img class="poster" src="${api(item.posterURL)}" alt="" loading="lazy">`
        : "";
      const copies = opts.copyCount != null ? opts.copyCount : (dedupEnabled ? copiesOf(item).length : 1);
      const h = copies > 1 ? copyHeight(item) : 0;
      const qualityBit = h ? (h >= 2160 ? "4K" : h + "p") : "";
      const bookContext = item.kind === "audiobook" ? [item.author, item.series, item.seriesNumber ? `#${item.seriesNumber}` : ""].filter(Boolean).join(" · ") : "";
      const metaBits = [bookContext, item.year || "", runtimeLabel(item), qualityBit].filter(Boolean).join(" • ");
      const badge = watched
        ? `<span class="badge watched">WATCHED</span>`
        : (pct > 0 ? `<span class="badge unwatched">${Math.round(100-pct)}% LEFT</span>` : "");
      return `
      <button class="card" data-id="${item.id}" data-action="open-detail"
              aria-label="${escapeHTML(item.title)}${metaBits ? ", " + metaBits : ""}">
        <div class="frame">
          ${poster}
          <span class="play-glyph" aria-hidden="true">▶</span>
          ${badge}
          ${pct > 0 ? `<span class="bar"><i style="width:${pct}%"></i></span>` : ""}
        </div>
        <h3>${escapeHTML(opts.showTitle ? (item.showTitle || item.title) : item.title)}</h3>
        <p class="meta">${escapeHTML(metaBits || kindLabel(item.kind))}</p>
      </button>`;
    }

    const PAGE_SIZE = 200;
    let currentPage = 1;
    let activeTab = "all";
    let openShow = null; // show name when drilled into a TV show
    let openSeason = null;
    let selectedMovieID = null;
    let movieDetailCollapsed = true;
    let movieShelfExpanded = false;
    let libraryShelfLimit = 96;
    const movieMetadataByID = new Map();
    const movieMetadataLoading = new Set();
    const movieMetadataErrors = new Map();

    function movieShelfGroups(source) {
      const movies = (source || []).filter(item => item.kind === "movie" && !item.isPlaceholder);
      const byUpdated = (a, b) => (progressByID.get(b.id)?.updatedAt || "")
        .localeCompare(progressByID.get(a.id)?.updatedAt || "");
      const fresh = movies.filter(item => !isWatched(progressFor(item.id), item) && !inProgress(progressFor(item.id), item))
        .sort((a, b) => (b.year || 0) - (a.year || 0) || a.title.localeCompare(b.title));
      return {
        continueWatching: movies.filter(item => inProgress(progressFor(item.id), item)).sort(byUpdated),
        featured: fresh,
        quick: movies.filter(item => item.durationSeconds > 0 && item.durationSeconds < 2 * 60 * 60),
        long: movies.filter(item => item.durationSeconds >= 2 * 60 * 60),
        full: movies,
      };
    }

    function movieSpotlight(source) {
      const groups = movieShelfGroups(source);
      return groups.continueWatching[0] || groups.featured[0] || groups.full[0] || null;
    }

    function buildShowGroups(visibleIDs = null) {
      const map = new Map();
      const seen = new Set();
      for (const i of items) {
        if (i.kind !== "tvShow" || i.isPlaceholder || !(i.showGroupTitle || i.showTitle)) continue;
        if (visibleIDs && !visibleIDs.has(i.id)) continue;
        const key = itemGroupKey.get(i.id) ?? copyKey(i);
        if (seen.has(key)) continue;
        seen.add(key);
        const representative = copiesOf(i)[0];
        const groupID = i.showGroupID || i.showTitle;
        let show = map.get(groupID);
        if (!show) {
          show = { id:groupID, name:i.showGroupTitle || i.showTitle, searchText:"", seasons:new Map(), posterItem:null };
          map.set(groupID, show);
        }
        show.searchText += ` ${i.showTitle || ""} ${i.title || ""} ${i.subtitle || ""}`;
        const sn = i.seasonNumber ?? 0;
        if (!show.seasons.has(sn)) show.seasons.set(sn, []);
        show.seasons.get(sn).push(representative);
        if (i.posterURL && (!show.posterItem || !show.posterItem.posterURL)) show.posterItem = i;
      }
      for (const show of map.values()) {
        for (const eps of show.seasons.values())
          eps.sort((a,b)=>(a.episodeNumber??0)-(b.episodeNumber??0)||a.title.localeCompare(b.title));
      }
      return Array.from(map.values()).sort((a,b)=>a.name.localeCompare(b.name));
    }

    function seasonLabel(n) {
      return n > 0 ? `Season ${String(n).padStart(2,"0")}` : "Specials / Extras";
    }

    // A card for a TV show folder: clicking drills in instead of opening the detail modal.
    function showCardHTML(show) {
      const epCount = Array.from(show.seasons.values()).reduce((n,e)=>n+e.length,0);
      let watchedCount = 0;
      for (const eps of show.seasons.values())
        for (const e of eps) if (isWatched(progressFor(e.id), e)) watchedCount++;
      const pct = epCount ? Math.round(watchedCount/epCount*100) : 0;
      const item = show.posterItem;
      const poster = item?.posterURL
        ? `<img class="poster" src="${api(item.posterURL)}" alt="" loading="lazy">`
        : "";
      const badge = pct >= 100
        ? `<span class="badge watched">WATCHED</span>`
        : (watchedCount > 0 ? `<span class="badge unwatched">${epCount-watchedCount} LEFT</span>` : "");
      return `
      <button class="card" data-show="${escapeHTML(show.id)}" data-action="open-show" aria-label="${escapeHTML(show.name)}">
        <div class="frame">${poster}<span class="play-glyph" aria-hidden="true">▶</span>${badge}</div>
        <h3>${escapeHTML(show.name)}</h3>
        <p class="meta">${show.seasons.size} season${show.seasons.size===1?"":"s"} • ${epCount} episode${epCount===1?"":"s"}</p>
      </button>`;
    }

    function episodeRowHTML(ep) {
      const p = progressFor(ep.id);
      const pct = (() => {
        if (!p || p.seconds <= 0) return 0;
        const dur = p.duration || ep.durationSeconds || 0;
        return dur > 0 ? Math.min(100, p.seconds/dur*100) : 0;
      })();
      const code = ep.seasonNumber != null && ep.episodeNumber != null
        ? `S${String(ep.seasonNumber).padStart(2,"0")}E${String(ep.episodeNumber).padStart(2,"0")}`
        : "—";
      return `
      <button class="ep-row" data-id="${ep.id}" data-action="open-detail" aria-label="${escapeHTML(code + " " + ep.title)}">
        <span class="code">${code}</span>
        <span class="eptitle">${escapeHTML(ep.title)}</span>
        <span class="runtime">${pct>0 ? (isWatched(p,ep) ? "Watched" : formatTime(p.seconds)+" / "+runtimeLabel(ep)) : runtimeLabel(ep)}</span>
        <span class="epbar"><i style="width:${pct}%"></i></span>
      </button>`;
    }

    function renderShowPage() {
      const groups = buildShowGroups();
      const show = groups.find(s => s.id === openShow) || groups.find(s => s.name === openShow);
      const grid = $("#grid");
      const pager = $("#pager");
      const seasons = $("#seasonList");
      $("#backRow").hidden = false;
      grid.className = "";
      pager.innerHTML = "";
      if (!show) {
        openShow = null;
        $("#backRow").hidden = true;
        seasons.innerHTML = "";
        render();
        return;
      }
      document.title = `${show.name} — TM Sonder`;
      const numbers = Array.from(show.seasons.keys()).sort((a,b)=>{
        if (a<=0 && b>0) return 1; if (b<=0 && a>0) return -1; return a-b;
      });
      grid.innerHTML = "";
      $("#continueRow").hidden = true;
      $("#backRow button").textContent = openSeason == null ? "‹ All Shows" : "‹ All Seasons";
      if (openSeason == null) {
        grid.className = "grid";
        seasons.innerHTML = "";
        grid.innerHTML = numbers.map(n => `<button class="card" data-action="open-season" data-season="${n}">
          <h3>${seasonLabel(n)}</h3><p class="meta">${show.seasons.get(n).length} episodes</p></button>`).join("");
        return;
      }
      seasons.innerHTML = numbers.filter(n => n === openSeason).map(n => `
        <section class="season">
          <h2>${seasonLabel(n)} — ${show.seasons.get(n).length} episode${show.seasons.get(n).length===1?"":"s"}</h2>
          ${show.seasons.get(n).map(episodeRowHTML).join("")}
        </section>`).join("")
        || `<div class="empty-state">No episodes found.</div>`;
    }

    function openShowPage(name) {
      openShow = name;
      openSeason = null;
      currentPage = 1;
      setTab("tvshows", true);
      render();
      syncHash(true);
      window.scrollTo({ top:0 });
    }
    function leaveShow() {
      if (openSeason != null) {
        openSeason = null;
        render();
        syncHash(true);
        return;
      }
      openShow = null;
      document.title = "TM Sonder";
      $("#backRow").hidden = true;
      $("#seasonList").innerHTML = "";
      render();
      syncHash(false);
    }

    // --- URL state (#tab[/show/<name>|/page/<n>]) so refresh and Back work ---
    function syncHash(push) {
      const parts = [activeTab];
      if (["audiobooks", "books"].includes(activeTab) && selectedListID) {
        parts.push("list", selectedListID);
      } else if (openShow) {
        parts.push("show", openShow);
        if (openSeason != null) parts.push("season", String(openSeason));
      }
      else if (currentPage > 1) parts.push("page", String(currentPage));
      const h = "#" + parts.map(encodeURIComponent).join("/");
      if (location.hash === h) return;
      if (push) history.pushState(null, "", h);
      else history.replaceState(null, "", h);
    }
    function applyHash() {
      const seg = location.hash.replace(/^#\/?/, "").split("/")
        .filter(s => s !== "").map(decodeURIComponent);
      if (seg.length === 0) return false;
      const tabs = ["all", "movies", "tvshows", "documentaries", "audiobooks", "books", "storage", "optimize"];
      // Keep old #lists links useful after Lists moved into the reading tabs.
      const requestedTab = seg[0] === "lists" ? "books" : seg[0];
      const tab = tabs.includes(requestedTab) && !tabIsHidden(requestedTab) ? requestedTab : "all";
      activeTab = tab;
      for (const b of document.querySelectorAll("#tabs button"))
        b.classList.toggle("active", b.dataset.tab === tab);
      openShow = null;
      openSeason = null;
      selectedListID = null;
      currentPage = 1;
      if (seg[1] === "show" && seg[2]) openShow = seg[2];
      if (seg[0] === "lists" && seg[1] === "list" && seg[2]) selectedListID = seg[2];
      if (openShow && seg[3] === "season" && /^\d+$/.test(seg[4] || "")) openSeason = Number(seg[4]);
      else if (seg[1] === "page") currentPage = Math.max(1, parseInt(seg[2], 10) || 1);
      return true;
    }
    window.addEventListener("popstate", () => {
      applyHash();
      if (!openShow) {
        $("#backRow").hidden = true;
        $("#seasonList").innerHTML = "";
        if (document.title !== "TM Sonder") document.title = "TM Sonder";
      }
      render();
      if (activeTab === "optimize") {
        refreshOptimizationQueue();
        refreshAudiobookJobs();
      } else if (optimizationPollTimer) {
        clearTimeout(optimizationPollTimer);
        optimizationPollTimer = null;
      }
    });

    function setTab(tab, keepShow=false) {
      const previousTab = activeTab;
      activeTab = tab;
      libraryShelfLimit = 96;
      if (tab !== previousTab) selectedListID = null;
      if (!keepShow) { openSeason = null; leaveShow(); }
      for (const b of document.querySelectorAll("#tabs button")) {
        b.classList.toggle("active", b.dataset.tab === tab);
      }
      if (tab !== "optimize" && optimizationPollTimer) {
        clearTimeout(optimizationPollTimer);
        optimizationPollTimer = null;
      }
    }

    for (const b of document.querySelectorAll("#tabs button")) {
      b.addEventListener("click", () => {
        currentPage = 1;
        setTab(b.dataset.tab);
        // Facets are tab-scoped, so a selection from another tab is stale.
        selectedFacetValues.clear();
        facetQuery = "";
        chipsExpanded = false;
        genreCategory = null;
        const search = $("#facetSearch");
        if (search) search.value = "";
        renderFacets();
        render();
        if (b.dataset.tab === "optimize") {
          refreshOptimizationQueue();
          refreshAudiobookJobs();
        }
        syncHash(true);
        window.scrollTo({ top:0 });
      });
    }

    async function refreshLists() {
      const response = await fetch(api("/api/lists"));
      if (!response.ok) throw new Error("Lists unavailable");
      lists = (await response.json()).lists || [];
      if (["audiobooks", "books"].includes(activeTab)) renderLists();
    }

    async function listMutation(path, options) {
      const response = await fetch(api(path), options);
      if (!response.ok) throw new Error((await response.text()) || "List update failed");
      await refreshLists();
    }

    // Media managers name files "Title (Year)", and the scanner keeps that
    // suffix as the title. Matching a curated "Title (Year)" entry against a
    // catalog "Title (Year)" therefore has to drop the suffix from the
    // candidate side first, or every movie match falls through to the loose
    // containment path below and the year check stops meaning anything.
    function listRecordParts(rawTitle, rawYear) {
      const title = String(rawTitle || "");
      const match = CURATED_TITLE_YEAR.exec(title);
      return {
        title: match ? match[1].trim() : title,
        year: Number(rawYear) || (match ? Number(match[2]) : 0),
      };
    }

    // Containment is a last resort, so it needs enough shared text to mean
    // something. Without a floor, a one-letter entry like "M" matches any
    // title containing an "m" — which is how a bonus-feature extra ends up on
    // a greatest-films shelf.
    const LOOSE_MATCH_MIN = 5;
    function looseTitleMatch(wanted, key) {
      return Math.min(wanted.length, key.length) >= LOOSE_MATCH_MIN &&
        (key.includes(wanted) || wanted.includes(key));
    }

    // Every media kind in a curated list needs its own candidate pool, because
    // a shelf for movies must never match a documentary with the same title.
    // The pool key is the sorted kind list, and TV pools are built from show
    // groups rather than episodes so a list entry names a series, not S01E01.
    function listCandidateIndex(kinds = CURATED_KINDS.book) {
      const wanted = [...kinds].sort();
      const signature = wanted.join(",");
      const cached = listCandidateState.get(signature);
      if (cached && cached.items === items && cached.generation === listIndexGeneration) return cached;

      const records = [];
      if (wanted.includes("tvShow")) {
        for (const show of buildShowGroups()) {
          const item = show.posterItem || [...show.seasons.values()].flat()[0];
          if (!item) continue;
          const parts = listRecordParts(show.name, 0);
          records.push({ item, show, ...parts, key: listTitleKey(parts.title) });
        }
      }
      for (const kind of wanted) {
        if (kind === "tvShow") continue;
        for (const item of items) {
          if (item.kind !== kind || item.isPlaceholder) continue;
          const parts = listRecordParts(item.title, item.year);
          records.push({ item, show: null, ...parts, key: listTitleKey(parts.title) });
        }
      }
      records.sort((a, b) => String(a.title || "").localeCompare(String(b.title || "")));

      // Exact-title buckets hold one record per year so a year-aware lookup
      // resolves a remake without scanning, and a title-only lookup still finds
      // something when the catalog has the film under an unrecorded year.
      const exact = new Map();
      for (const record of records) {
        if (!record.key) continue;
        if (!exact.has(record.key)) exact.set(record.key, { any: null, byYear: new Map() });
        const bucket = exact.get(record.key);
        if (record.year) {
          if (!bucket.byYear.has(record.year)) bucket.byYear.set(record.year, record);
          else if (!bucket.any) bucket.any = record;
        } else if (!bucket.any) {
          bucket.any = record;
        }
      }
      const candidates = records.map(record => record.item);
      const state = {
        items, generation: listIndexGeneration, signature, candidates, records, exact, matches: new Map(),
      };
      listCandidateState.set(signature, state);
      return state;
    }

    // listCandidates keeps its historical meaning: the pool of items the current
    // tab's lists may add. An unfiltered tab (home) offers every kind.
    function listCandidates(kinds = tabCatalogKinds()) {
      return listCandidateIndex(kinds.length ? kinds : allListKinds()).candidates;
    }

    function allListKinds() {
      return Object.values(CURATED_KINDS).flat();
    }

    // A curated entry resolves to a candidate record. `entry` is either a parsed
    // {title, year} or a bare title string from a hand-written list.
    function candidateForEntry(entry, kinds) {
      const parsed = typeof entry === "string" ? parseCuratedEntry(entry) : entry;
      if (!parsed?.title) return null;
      const state = listCandidateIndex(kinds);
      const wanted = listTitleKey(parsed.title);
      const cacheKey = wanted + "|" + (parsed.year || 0);
      if (state.matches.has(cacheKey)) return state.matches.get(cacheKey) || null;

      let match = null;
      const bucket = state.exact.get(wanted);
      if (bucket) {
        if (parsed.year) {
          // Prefer the same year, then a catalog entry with no year at all.
          // A different known year is a different film, not a near miss.
          match = bucket.byYear.get(parsed.year) || (bucket.any && !bucket.any.year ? bucket.any : null);
        } else {
          match = bucket.any || [...bucket.byYear.values()][0] || null;
        }
      }
      if (!match && wanted) {
        const loose = state.records.find(record =>
          looseTitleMatch(wanted, record.key) &&
          (!parsed.year || !record.year || record.year === parsed.year));
        match = loose || null;
      }
      state.matches.set(cacheKey, match || false);
      return match;
    }

    // Kept for the reading-list UI, which still speaks in bare titles.
    function candidateForListTitle(title, candidates, kinds) {
      const state = listCandidateIndex(kinds);
      const pool = candidates || state.candidates;
      if (pool !== state.candidates) {
        const wanted = listTitleKey(title);
        return pool.find(item => {
          const actual = listTitleKey(item.title);
          return actual === wanted || actual.includes(wanted) || wanted.includes(actual);
        });
      }
      return candidateForEntry(parseCuratedEntry(title), kinds)?.item || null;
    }

    function updateListBookChoices(input) {
      const card = input.closest("[data-list-id]");
      const select = card?.querySelector("[data-list-select]");
      if (!select) return;
      const wanted = listTitleKey(input.value);
      if (!wanted) {
        select.innerHTML = '<option value="">Type to search for a title…</option>';
        select.disabled = true;
        return;
      }
      const matches = listCandidateIndex(listPoolKinds()).records
        .filter(record => record.key.includes(wanted))
        .slice(0, 80)
        .map(record => record.item);
      select.innerHTML = matches.length
        ? `<option value="">Choose a matching title…</option>${matches.map(item => `<option value="${escapeHTML(item.id)}">${escapeHTML(item.title || item.id)}</option>`).join("")}`
        : '<option value="">No matching titles</option>';
      select.disabled = matches.length === 0;
    }

    // The kinds the add-row and the shelf picker should offer right now. A list
    // can hold any kind, so an existing list keeps the pool of whatever catalog
    // it was opened from; the home tab offers everything.
    function listPoolKinds() {
      const kinds = tabCatalogKinds();
      return kinds.length ? kinds : allListKinds();
    }

    // ── Curated shelves as discovery rails ─────────────────────────────────
    // The curated data used to be an admin surface: you could save a shelf but
    // never browse one. A shelf earns a rail once the library already holds
    // enough of it to be worth looking at — that is the interesting case,
    // because the titles still missing become an obvious shopping list. The
    // rail keeps the list's own ranking, which is the whole point of a canon:
    // the top of a "greatest films" shelf is not its first letter.
    //
    // Many book shelves are the same 100 titles under different publications'
    // names. They collapse to one rail, because three identically-worded
    // "67 of 100 owned" shelves is noise rather than choice.
    const CURATED_RAIL_MIN = 4;
    const CURATED_RAIL_MAX = 2;

    function curatedShelves(kinds, limit = CURATED_RAIL_MAX) {
      const shelves = curatedListsForKinds(kinds)
        .map(list => {
          const records = list.entries.map(entry => candidateForEntry(entry, kinds)).filter(Boolean);
          return { list, records, missing: list.entries.length - records.length };
        })
        .filter(shelf => shelf.records.length >= CURATED_RAIL_MIN);
      // Most covered first, then by name so a shelf cannot reorder between
      // renders just because two lists tie on coverage.
      shelves.sort((a, b) => b.records.length - a.records.length || a.list.name.localeCompare(b.list.name));
      const seen = new Set();
      return shelves.filter(shelf => {
        if (seen.has(shelf.list.signature)) return false;
        seen.add(shelf.list.signature);
        return true;
      }).slice(0, limit);
    }

    function curatedShelfSub(shelf) {
      const owned = shelf.records.length;
      const total = shelf.list.entries.length;
      return shelf.missing
        ? `${owned} of ${total} owned · ${shelf.missing} still to find`
        : `all ${total} of this shelf is in your library`;
    }

    function curatedShelfControl(shelf) {
      const saved = lists.some(list => list.name === shelf.list.name);
      return `<button type="button" class="rail-browse" data-action="use-recommended-list" data-recommended-id="${escapeHTML(shelf.list.id)}">${saved ? "Saved · open" : "Save shelf"}</button>`;
    }

    function curatedShelfCards(shelf, limit = 18) {
      return shelf.records
        .slice(0, limit)
        .map(record => (record.show ? showCardHTML(record.show) : cardHTML(record.item)));
    }

    // The curated shelf picker only offers lists that can match the tab's own
    // catalog, so the Movies tab never proposes a book shelf and a film's
    // coverage count is measured against movies alone.
    function curatedListsForTab() {
      const kinds = tabCatalogKinds();
      return kinds.length ? curatedListsForKinds(kinds) : CURATED_LISTS;
    }

    function renderLists() {
      const host = $("#listsView");
      if (!host) return;
      const kinds = listPoolKinds();
      const candidates = listCandidates(kinds);
      const poolLabel = listPoolLabel(kinds);
      const curated = curatedListsForTab();
      const selectedList = selectedListID ? lists.find(list => list.id === selectedListID) : null;
      const recommended = selectedList ? "" : `<section class="recommended-lists"><div class="recommended-lists-head"><div><h3>Curated shelves for ${escapeHTML(poolLabel)}</h3><p class="muted">Click any card to add that ordered shelf to Sonder, ordered as the list ranks it.</p></div><span class="muted">${curated.length} lists</span></div><div class="recommended-list-grid">${curated.map(list => {
        const present = list.entries.filter(entry => candidateForEntry(entry, kinds)).length;
        const alreadyAdded = lists.some(existing => existing.name === list.name);
        return `<button class="recommended-list-card" data-action="use-recommended-list" data-recommended-id="${escapeHTML(list.id)}"><span class="recommended-list-icon">▦</span><strong>${escapeHTML(list.name)}</strong><span class="muted">${escapeHTML(list.description)}</span><span class="recommended-list-meta">${present}/${list.entries.length} in library · ${alreadyAdded ? "Added" : "Add list"}</span></button>`;
      }).join("")}</div></section>`;
      const savedLists = selectedList ? [selectedList] : lists;
      const saved = savedLists.map(list => {
        const entries = (list.items || []).map((entry, index) => {
          const item = entry.item || {};
          const cover = item.posterURL ? `<img class="list-entry-cover" src="${escapeHTML(api(item.posterURL))}" alt="" loading="lazy">` : `<span class="list-entry-cover list-entry-cover-empty" aria-hidden="true">▧</span>`;
          const tags = (entry.tags || []).map(tag => `<span class="tag">${escapeHTML(tag)}</span>`).join("");
          return `<li>${cover}<span class="list-position">${index + 1}.</span><button class="list-entry-title" data-action="open-detail" data-id="${escapeHTML(item.id || "")}">${escapeHTML(item.title || item.id || "Unknown title")}</button><span class="list-entry-tags">${tags}</span><span class="list-entry-actions"><button data-action="move-list-item" data-list-id="${escapeHTML(list.id)}" data-index="${index}" data-direction="up" ${index === 0 ? "disabled" : ""}>↑</button><button data-action="move-list-item" data-list-id="${escapeHTML(list.id)}" data-index="${index}" data-direction="down" ${index === list.items.length - 1 ? "disabled" : ""}>↓</button><button data-action="remove-list-item" data-list-id="${escapeHTML(list.id)}" data-item-id="${escapeHTML(item.id || "")}">Remove</button></span></li>`;
        }).join("");
        const addRow = selectedList
          ? `<div class="list-add-row"><input class="list-book-search" data-list-book-search placeholder="Search ${escapeHTML(poolLabel.toLowerCase())} to add…" aria-label="Search titles to add"><select data-list-select aria-label="Title to add" disabled><option value="">Type to search for a title…</option></select><input data-list-tags placeholder="Entry tags, comma separated" aria-label="Entry tags"><button class="primary" data-action="add-list-item" data-list-id="${escapeHTML(list.id)}">Add</button></div>`
          : "";
        const listTags = (list.tags || []).map(tag => `<span class="tag">${escapeHTML(tag)}</span>`).join("");
        const recommendation = CURATED_LISTS.find(candidate => candidate.name === list.name);
        const missing = recommendation ? recommendation.entries.filter(entry => !candidateForEntry(entry, recommendation.kinds)) : [];
        const missingHTML = missing.length ? `<section class="list-missing"><div><strong>${missing.length} missing titles</strong><p class="muted">Not currently anywhere in your ${escapeHTML(listPoolLabel(recommendation.kinds).toLowerCase())}.</p></div><button data-action="export-missing" data-list-id="${escapeHTML(list.id)}">Export missing .txt</button><ol>${missing.map(entry => `<li>${escapeHTML(entry.year ? `${entry.title} (${entry.year})` : entry.title)}</li>`).join("")}</ol></section>` : "";
        const title = selectedList
          ? `<h3>${escapeHTML(list.name)}</h3>`
          : `<h3><button class="list-open-title" data-action="open-reading-list" data-list-id="${escapeHTML(list.id)}">${escapeHTML(list.name)}</button></h3>`;
        return `<article class="reading-list${selectedListID === list.id ? " selected-reading-list" : ""}" data-list-id="${escapeHTML(list.id)}"><div class="reading-list-head"><div>${title}${list.description ? `<p>${escapeHTML(list.description)}</p>` : ""}<div>${listTags}</div></div><button data-action="delete-list" data-list-id="${escapeHTML(list.id)}">Delete</button></div>${addRow}<ol>${entries || '<li class="list-empty">No titles yet.</li>'}</ol>${missingHTML}</article>`;
      }).join("");
      const detailHeader = selectedList ? `<div class="list-detail-header"><button data-action="back-to-lists">‹ All lists</button><span class="muted">Dedicated list page</span></div>` : "";
      host.innerHTML = detailHeader + recommended + (saved || '<div class="empty-state">Create a list to start building a queue.</div>');
    }

    function listPoolLabel(kinds) {
      const names = (kinds || []).map(kindLabel);
      if (!names.length) return "this catalog";
      // The home tab pools every kind; naming all five would be noise.
      if (names.length > 2) return "your whole library";
      if (names.length === 1) return names[0];
      return names[0] + " and " + names[1];
    }

    function exportMissingList(listID) {
      const list = lists.find(candidate => candidate.id === listID);
      const recommendation = list && CURATED_LISTS.find(candidate => candidate.name === list.name);
      if (!recommendation) return;
      const missing = recommendation.entries.filter(entry => !candidateForEntry(entry, recommendation.kinds));
      const label = (entry) => (entry.year ? `${entry.title} (${entry.year})` : entry.title);
      const body = [`${recommendation.name} — missing titles`, "", ...missing.map((entry, index) => `${index + 1}. ${label(entry)}`), ""].join("\n");
      const link = document.createElement("a");
      link.href = URL.createObjectURL(new Blob([body], { type: "text/plain;charset=utf-8" }));
      link.download = `${recommendation.name.replace(/[^a-z0-9]+/gi, "-").toLowerCase()}-missing.txt`;
      link.click();
      URL.revokeObjectURL(link.href);
    }

    // Materializing a curated shelf keeps the list's own ranking, so a saved
    // shelf is a queue rather than an alphabetical dump.
    async function useRecommendedList(recommendedID) {
      const recommendation = CURATED_LISTS.find(list => list.id === recommendedID);
      if (!recommendation) return;
      const existing = lists.find(list => list.name === recommendation.name);
      if (existing) {
        selectedListID = existing.id;
        syncHash(true);
        renderLists();
        return;
      }
      try {
        const created = await fetch(api("/api/lists"), { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ name: recommendation.name, description: recommendation.description, tags: ["recommended", "curated"] }) });
        if (!created.ok) throw new Error((await created.text()) || "Could not create recommendation list");
        const list = await created.json();
        let position = 0;
        for (const entry of recommendation.entries) {
          const record = candidateForEntry(entry, recommendation.kinds);
          if (!record) continue;
          const response = await fetch(api(`/api/lists/${encodeURIComponent(list.id)}/items`), { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ itemID: record.item.id, position, tags: ["recommended"] }) });
          if (response.ok) position++;
        }
        selectedListID = list.id;
        syncHash(true);
        await refreshLists();
      } catch (error) { alert(error.message || "Could not add recommendation list"); }
    }

    on("#listCreateForm", "submit", async e => {
      e.preventDefault();
      const tags = $("#newListTags").value.split(",").map(value => value.trim()).filter(Boolean);
      try {
        await listMutation("/api/lists", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ name: $("#newListName").value, description: $("#newListDescription").value, tags }) });
        e.target.reset();
      } catch (error) { alert(error.message || "Could not create list"); }
    });

    function movieMetaInitials(name) {
      return String(name || "?")
        .split(/\s+/)
        .filter(Boolean)
        .slice(0, 2)
        .map(part => part[0])
        .join("")
        .toUpperCase() || "?";
    }

    function movieMetadataMarkup(item) {
      const metadata = movieMetadataByID.get(item.id);
      if (movieMetadataLoading.has(item.id)) {
        return `<section class="movie-meta-loading" aria-live="polite"><span class="catalog-kicker">LOOKING CLOSER</span><p>Loading cast, characters, and ratings…</p></section>`;
      }
      if (movieMetadataErrors.has(item.id)) {
        return `<section class="movie-meta-loading movie-meta-error"><span class="catalog-kicker">LOCAL METADATA</span><p>Online cast and rating details are unavailable right now. The local primer above is still ready.</p></section>`;
      }
      if (!metadata) return "";

      const directors = (metadata.directors || []).filter(Boolean);
      const genres = (metadata.genres || []).filter(Boolean).slice(0, 8);
      const ratings = (metadata.ratings || []).filter(rating => rating && rating.value);
      const cast = (metadata.cast || []).filter(member => member && member.name).slice(0, 10);
      const creditBits = [];
      if (directors.length) creditBits.push(`<span><strong>Directed by</strong>${escapeHTML(directors.join(", "))}</span>`);
      if (genres.length) creditBits.push(`<span><strong>Genres</strong>${genres.map(genre => escapeHTML(genre)).join(" · ")}</span>`);
      const creditsHTML = creditBits.length ? `<div class="movie-credits">${creditBits.join("")}</div>` : "";
      const ratingsHTML = ratings.length ? `
        <section class="movie-ratings" aria-label="Movie ratings">
          <div class="movie-section-head"><span class="catalog-kicker">RATINGS</span><span>Published snapshots</span></div>
          <div class="movie-rating-grid">${ratings.map(rating => `
            <div class="movie-rating"><strong>${escapeHTML(rating.value)}</strong><span>${escapeHTML(rating.source || "Wikidata")}</span>${rating.method ? `<small>${escapeHTML(rating.method)}</small>` : ""}</div>`).join("")}</div>
        </section>` : "";
      const castHTML = cast.length ? `
        <section class="movie-cast" aria-label="Main cast">
          <div class="movie-section-head"><span class="catalog-kicker">CAST</span><span>Main players</span></div>
          <div class="movie-cast-grid">${cast.map(member => `
            <div class="movie-cast-member">
              <div class="movie-cast-portrait">${member.imageURL ? `<img src="${escapeHTML(member.imageURL)}" alt="" loading="lazy">` : `<span>${escapeHTML(movieMetaInitials(member.name))}</span>`}</div>
              <strong>${escapeHTML(member.name)}</strong>
              ${member.character ? `<span>${escapeHTML(member.character)}</span>` : `<span class="movie-cast-unknown">Character not indexed</span>`}
            </div>`).join("")}</div>
        </section>` : `<p class="movie-meta-muted">No cast details are indexed for this title yet.</p>`;
      const sourceHTML = metadata.wikiURL ? `<a class="movie-meta-source" href="${escapeHTML(metadata.wikiURL)}" target="_blank" rel="noopener">Metadata source: Wikipedia / Wikidata ↗</a>` : "";
      return `${creditsHTML}${ratingsHTML}${castHTML}${sourceHTML}`;
    }

    function requestMovieMetadata(item) {
      if (!item || movieMetadataByID.has(item.id) || movieMetadataLoading.has(item.id) || movieMetadataErrors.has(item.id)) return;
      movieMetadataLoading.add(item.id);
      fetch(api(`/api/movies/${encodeURIComponent(item.id)}/metadata`), { cache: "no-store" })
        .then(async response => {
          if (!response.ok) throw new Error(`metadata ${response.status}`);
          return response.json();
        })
        .then(metadata => movieMetadataByID.set(item.id, metadata || {}))
        .catch(error => {
          console.warn("Movie metadata unavailable", error);
          movieMetadataErrors.set(item.id, true);
        })
        .finally(() => {
          movieMetadataLoading.delete(item.id);
          if (activeTab === "movies" && selectedMovieID === item.id && !$("#movieCatalog")?.hidden) {
            renderMovieCatalog(visibleItems());
          }
        });
    }

    function movieDetailMarkup(item) {
      const p = progressFor(item.id);
      const plan = playbackPlan(item);
      const resumeAt = p && p.seconds > 5 ? p.seconds : 0;
      const isCurrent = nowPlayingItem && nowPlayingItem.id === item.id;
      const media = npMedia();
      const metadata = movieMetadataByID.get(item.id);
      const playingNow = isCurrent && media && !media.paused;
      const playLabel = playingNow ? "Pause" : resumeAt ? `Resume at ${formatTime(resumeAt)}` : "Play movie";
      const title = item.title || "Untitled movie";
      const poster = item.posterURL
        ? `<img src="${escapeHTML(api(item.posterURL))}" alt="">`
        : escapeHTML(title.slice(0, 1).toUpperCase() || "M");
      const meta = [item.year || "", runtimeLabel(item), item.probedHeight ? `${item.probedWidth || "?"}×${item.probedHeight}` : ""]
        .filter(Boolean).join(" · ");
      const tags = (item.tags || []).slice(0, 8)
        .map(tag => `<span class="tag">${escapeHTML(tag)}</span>`).join("");
      const summary = [metadata?.primer || item.summary, metadata?.primer ? "" : item.subtitle].filter(Boolean)
        .map(value => `<p class="summary">${escapeHTML(value)}</p>`).join("");
      const progress = p && p.seconds > 5
        ? `<p class="catalog-detail-meta">${isWatched(p, item) ? "Watched" : `Started · ${formatTime(p.seconds)}${item.durationSeconds ? ` of ${formatTime(item.durationSeconds)}` : ""}`}</p>`
        : "";
      return `
        <div class="catalog-detail-hero">
          <div class="catalog-detail-cover">${poster}</div>
          <div>
            <span class="pill">Movie</span>
            <h2>${escapeHTML(title)}</h2>
            <p class="catalog-detail-meta">${escapeHTML(meta || "Ready to watch")}</p>
            ${progress}
          </div>
        </div>
        ${plan ? `<div class="actions"><button class="primary" data-action="play-item" data-id="${escapeHTML(item.id)}">${playLabel}</button><a href="${api("/stream/" + item.id)}" target="_blank" rel="noopener">Open stream URL</a></div>` : `<div class="not-playable"><strong>.${escapeHTML(String(item.format || "?").toUpperCase())}</strong> can't play in the browser. <a href="${api("/stream/" + item.id)}" target="_blank" rel="noopener">Open in a native player</a>.</div>`}
        ${summary || `<p class="catalog-detail-empty">No synopsis is available for this movie yet.</p>`}
        ${movieMetadataMarkup(item)}
        ${tags ? `<div class="tagrow">${tags}</div>` : ""}
        ${editionsHTML(item)}`;
    }

    // The movie catalog has its own renderer, so it needs its own curated
    // rails rather than sharing the generic one.
    function curatedMovieShelves() {
      return curatedShelves(CURATED_KINDS.movie)
        .map(shelf => {
          const cards = shelf.records.slice(0, 18).map(record => cardHTML(record.item)).join("");
          return `<section class="web-rail">
            <div class="web-rail-heading">
              <div><h2>${escapeHTML(shelf.list.name)}</h2><p class="sub">${escapeHTML(curatedShelfSub(shelf))}</p></div>
              ${curatedShelfControl(shelf)}
            </div>
            <div class="catalog-strip">${cards}</div>
          </section>`;
        })
        .join("");
    }

    function renderMovieCatalog(source) {
      const layout = $("#movieCatalog");
      const rails = $("#movieRails");
      const detail = $("#movieDetailBody");
      if (!layout || !rails || !detail) return;
      const groups = movieShelfGroups(source);
      const selected = groups.full.find(item => item.id === selectedMovieID) || movieSpotlight(source);
      selectedMovieID = selected?.id || null;
      const shelf = (title, subtitle, list, full = false) => list.length ? `
        <section class="web-rail">
          <h2>${escapeHTML(title)}</h2><p class="sub">${escapeHTML(subtitle)}</p>
          <div class="${full ? "catalog-shelf" : "catalog-strip"}">${list.map(cardHTML).join("")}</div>
        </section>` : "";
      const fullShelf = movieShelfExpanded ? groups.full : groups.full.slice(0, 96);
      const fullShelfToggle = groups.full.length > fullShelf.length
        ? `<div class="catalog-more"><button type="button" data-action="expand-movie-shelf">Show all ${groups.full.length.toLocaleString()} movies</button></div>`
        : movieShelfExpanded && groups.full.length > 96
          ? `<div class="catalog-more"><button type="button" data-action="collapse-movie-shelf">Show a faster shelf</button></div>`
          : "";
      rails.innerHTML = shelf("Continue watching", `${groups.continueWatching.length} movies in progress`, groups.continueWatching.slice(0, 12)) +
        shelf("Featured movies", "newest unwatched films from your shelf", groups.featured.slice(0, 18)) +
        curatedMovieShelves() +
        shelf("Quick watches", "under two hours", groups.quick.slice(0, 18)) +
        shelf("Long-form cinema", "two hours and up", groups.long.slice(0, 18)) +
        shelf("Full movie shelf", `${groups.full.length.toLocaleString()} movies in your catalog`, fullShelf, true) +
        fullShelfToggle +
        (groups.full.length ? "" : `<p class="empty-state">No movies found in this catalog.</p>`);
      detail.innerHTML = selected
        ? movieDetailMarkup(selected)
        : `<p class="catalog-detail-empty">Choose a movie to see its story and start watching.</p>`;
      requestMovieMetadata(selected);
      layout.classList.toggle("detail-collapsed", movieDetailCollapsed);
      const toggle = $("#movieDetailToggle");
      if (toggle) {
        toggle.textContent = movieDetailCollapsed ? "Expand" : "Collapse";
        toggle.setAttribute("aria-expanded", String(!movieDetailCollapsed));
        toggle.setAttribute("aria-label", movieDetailCollapsed ? "Expand movie detail panel" : "Collapse movie detail panel");
      }
    }

    function openMovieDetail(id) {
      if (!items.some(item => item.id === id)) return;
      selectedMovieID = id;
      movieDetailCollapsed = false;
      renderMovieCatalog(visibleItems());
    }

    function toggleMovieDetail() {
      movieDetailCollapsed = !movieDetailCollapsed;
      renderMovieCatalog(visibleItems());
    }

    // Lists and curated shelves are a whole-catalog feature, not a books-only
    // one. The panel stays open on every browsing tab; Storage and Optimize are
    // the only pages that have no use for a queue.
    function renderReadingLists() {
      const panel = $("#listsPanel");
      if (!panel) return;
      const visible = !["storage", "optimize"].includes(activeTab);
      panel.hidden = !visible;
      if (visible) renderLists();
    }

    function render() {
      const storage = activeTab === "storage";
      const optimize = activeTab === "optimize";
      const coverFilter = $("#coverFilter");
      if (coverFilter) coverFilter.hidden = activeTab !== "audiobooks";
      document.body.classList.toggle("storage-mode", storage);
      document.body.classList.toggle("optimize-mode", optimize);
      $("#storagePanel").hidden = !storage;
      renderReadingLists();
      $("#optimizationPage").hidden = !optimize;
      $("#grid").hidden = storage || optimize;
      $("#pager").hidden = storage || optimize;
      $("#seasonList").hidden = storage || optimize;
      const movieCatalog = $("#movieCatalog");
      if (movieCatalog) movieCatalog.hidden = true;
      if (storage) { renderStorage(); return; }
      if (optimize) {
        $("#continueRow").hidden = true;
        $("#backRow").hidden = true;
        return;
      }
      $("#railsRow").hidden = true;
      if (openShow) { renderShowPage(); return; }

      const visible = visibleItems();
      const list = activeTab === "all" ? mainPageEntries(visible) : visible;
      // Rails is the shared home/library experience. Search, facets, and
      // watched filters intentionally fall back to the complete result grid
      // so every matching item remains easy to inspect. Classic skips the
      // shelf rows and keeps the original grid-first browser.
      const railsActive = libraryLayout === "rails" &&
        ["all", "movies", "tvshows", "documentaries", "audiobooks", "books"].includes(activeTab) &&
        $("#q").value.trim() === "" && selectedFacetValues.size === 0 &&
        $("#watched").value === "all" && !openShow;
      const railsRow = $("#railsRow");
      railsRow.hidden = true;
      if (railsActive && activeTab === "movies" && movieCatalog) {
        $("#continueRow").hidden = true;
        $("#grid").hidden = true;
        $("#pager").hidden = true;
        $("#seasonList").hidden = true;
        renderMovieCatalog(visible);
        movieCatalog.hidden = false;
        return;
      }
      if (railsActive) {
        const byUpdated = (a, b) => (progressByID.get(b.id)?.updatedAt || "")
          .localeCompare(progressByID.get(a.id)?.updatedAt || "");
        const shelfHeading = (title, sub, control = "") => `
          <div class="web-rail-heading">
            <div><h2>${escapeHTML(title)}</h2><p class="sub">${escapeHTML(sub)}</p></div>
            ${control}
          </div>`;
        const railStrip = (title, sub, cards, control = "") => cards.length ? `
          <section class="web-rail">${shelfHeading(title, sub, control)}
          <div class="grid rail-mode">${cards.join("")}</div></section>` : "";
        const browseAll = (tab, label = "Browse all") =>
          `<button type="button" class="rail-browse" data-action="browse-tab" data-tab="${escapeHTML(tab)}">${label}</button>`;
        // Curated rails, in one place, so every catalog gets the same treatment
        // and a shelf that is already saved says so instead of offering again.
        const curatedRails = (kinds, count = 2) => curatedShelves(kinds, count)
          .map(shelf => railStrip(shelf.list.name, curatedShelfSub(shelf), curatedShelfCards(shelf), curatedShelfControl(shelf)))
          .join("");
        const fullShelf = (title, sub, cards, renderCard) => {
          if (!cards.length) return "";
          const shown = cards.slice(0, libraryShelfLimit);
          const toggle = cards.length > shown.length
            ? `<div class="catalog-more"><button type="button" data-action="expand-library-shelf">Show more · ${shown.length.toLocaleString()} of ${cards.length.toLocaleString()}</button></div>`
            : libraryShelfLimit > 96
              ? `<div class="catalog-more"><button type="button" data-action="collapse-library-shelf">Show less</button></div>`
              : "";
          return `<section class="web-rail">${shelfHeading(title, sub)}
            <div class="catalog-shelf">${shown.map(renderCard).join("")}</div>${toggle}</section>`;
        };
        const fresh = source => source
          .filter(i => !isWatched(progressFor(i.id), i) && !inProgress(progressFor(i.id), i))
          .sort((a, b) => (b.year || 0) - (a.year || 0) || (a.title || "").localeCompare(b.title || ""));
        const showGroups = source => buildShowGroups(new Set(source.filter(i => i.kind === "tvShow").map(i => i.id)));
        const tabLabel = ({ documentaries:"Documentaries", audiobooks:"Audiobooks", books:"Books" })[activeTab] || "Library";
        let blocks = "";
        if (activeTab === "all") {
          const cont = visible.filter(i => inProgress(progressFor(i.id), i)).sort(byUpdated).slice(0, 12);
          const movies = visible.filter(i => i.kind === "movie").slice(0, 12);
          const shows = showGroups(visible).slice(0, 12);
          const documentaries = visible.filter(i => i.kind === "documentary").slice(0, 12);
          const audiobooks = visible.filter(i => i.kind === "audiobook").slice(0, 12);
          const books = visible.filter(i => i.kind === "ebook").slice(0, 12);
          blocks = railStrip("Continue Watching", `${cont.length} in progress`, cont.map(cardHTML)) +
                   curatedRails(allListKinds()) +
                   railStrip("Movies", `${movies.length} in the shelf`, movies.map(cardHTML), browseAll("movies")) +
                   railStrip("TV Shows", `${shows.length} shows in the shelf`, shows.map(showCardHTML), browseAll("tvshows")) +
                   railStrip("Documentaries", `${documentaries.length} in the shelf`, documentaries.map(cardHTML), browseAll("documentaries")) +
                   railStrip("Audiobooks", `${audiobooks.length} titles in the shelf`, audiobooks.map(cardHTML), browseAll("audiobooks")) +
                   railStrip("Books", `${books.length} titles in the shelf`, books.map(cardHTML), browseAll("books"));
        } else if (activeTab === "tvshows") {
          const shows = showGroups(visible);
          const showProg = s => {
            let prog = null, started = false;
            for (const eps of s.seasons.values()) for (const e of eps) {
              const p = progressFor(e.id);
              if (inProgress(p, e)) { prog = p; started = true; }
            }
            return { prog, started };
          };
          const cont = shows.filter(s => showProg(s).started)
            .sort((a, b) => (showProg(b).prog?.updatedAt || "").localeCompare(showProg(a).prog?.updatedAt || "")).slice(0, 10);
          const unstarted = shows.filter(s => !showProg(s).started).slice(0, 12);
          blocks = railStrip("Continue Watching", `${cont.length} shows in progress`, cont.map(showCardHTML)) +
                   railStrip("Unstarted", "nothing played yet", unstarted.map(showCardHTML)) +
                   curatedRails(CURATED_KINDS.tv) +
                   fullShelf("All TV shows", `${shows.length.toLocaleString()} shows in your catalog`, shows, showCardHTML);
        } else {
          const cont = visible.filter(i => inProgress(progressFor(i.id), i)).sort(byUpdated).slice(0, 10);
          const picks = fresh(visible).slice(0, 12);
          const continueLabel = activeTab === "audiobooks" ? "Continue Listening" : activeTab === "books" ? "Continue Reading" : "Continue Watching";
          const upNextLabel = activeTab === "audiobooks" ? "Up Next" : activeTab === "books" ? "Next Reads" : "Unwatched Picks";
          const upNextSub = activeTab === "audiobooks" ? "unstarted titles from your shelves" : activeTab === "books" ? "unstarted books from your shelves" : "newest unwatched titles";
          blocks = railStrip(continueLabel, `${cont.length} in progress`, cont.map(cardHTML)) +
                   railStrip(upNextLabel, upNextSub, picks.map(cardHTML)) +
                   curatedRails(tabCatalogKinds()) +
                   fullShelf(`All ${tabLabel}`, `${visible.length.toLocaleString()} titles in your catalog`, visible, cardHTML);
        }
        if (blocks) {
          document.querySelector("#railsBlocks").innerHTML = blocks;
          railsRow.hidden = false;
          $("#continueRow").hidden = true;
          $("#grid").hidden = true;
          $("#pager").hidden = true;
          $("#seasonList").hidden = true;
          return;
        }
      }
      const continuing = items.filter(i =>
        ["movie","tvShow","documentary"].includes(i.kind) &&
        inProgress(progressFor(i.id), i))
        .sort((a,b)=>(progressByID.get(b.id)?.updatedAt || "").localeCompare(
                      progressByID.get(a.id)?.updatedAt || ""));

      $("#continueRow").hidden = libraryLayout === "rails" || continuing.length === 0 || activeTab !== "all";
      if (!$("#continueRow").hidden) {
        $("#continueGrid").innerHTML = continuing.map(i => cardHTML(i)).join("");
      }

      const grid = $("#grid");
      const pager = $("#pager");
      const seasons = $("#seasonList");
      seasons.innerHTML = "";

      // TV Shows tab: group episodes into clickable show folders.
      if (activeTab === "tvshows") {
        const visibleIDs = new Set(visible.filter(i => i.kind === "tvShow").map(i => i.id));
        let shows = buildShowGroups(visibleIDs);
        const term = $("#q").value.trim().toLowerCase();
        if (term) shows = shows.filter(s =>
          foldDiacritics(`${s.name} ${s.searchText}`).toLowerCase().includes(foldDiacritics(term)));
        grid.className = "grid";
        const episodeCount = shows.reduce((count, show) =>
          count + Array.from(show.seasons.values()).reduce((n, eps) => n + eps.length, 0), 0);
        pager.innerHTML = `<span class="pageinfo">${shows.length.toLocaleString()} show${shows.length===1?"":"s"} • ${episodeCount.toLocaleString()} episode${episodeCount===1?"":"s"}</span>`;
        if (shows.length === 0) {
          grid.innerHTML = `<div class="empty-state">No shows yet. Add a TV Shows library with Show Name/Season XX/Episode files.</div>`;
          return;
        }
        grid.innerHTML = shows.map(showCardHTML).join("");
        return;
      }

      // Kind tabs with no content get a specific hint instead of a generic
      // "no matches" (e.g. no Documentaries library configured).
      const tabHints = {
        documentaries: "No documentaries yet. Add a Documentaries library in Settings.",
        audiobooks: "No audiobooks yet. Add an Audiobooks library in Settings, or open the Audiobook player from the app menu.",
        books: "No books yet. Add a Books library in Settings.",
      };
      if (list.length === 0 && tabHints[activeTab]) {
        grid.className = "";
        grid.innerHTML = `<div class="empty-state">${tabHints[activeTab]}</div>`;
        pager.innerHTML = "";
        return;
      }

      if (list.length === 0) {
        grid.className = "";
        grid.innerHTML = `<div class="empty-state">No matches. Adjust filters or add media to your libraries.</div>`;
        pager.innerHTML = "";
        return;
      }
      grid.className = "grid";

      const totalPages = Math.max(1, Math.ceil(list.length / PAGE_SIZE));
      if (currentPage > totalPages) currentPage = totalPages;
      if (currentPage < 1) currentPage = 1;
      const pageItems = list.slice((currentPage-1)*PAGE_SIZE, currentPage*PAGE_SIZE);

      grid.innerHTML = pageItems.map(i => i.show ? showCardHTML(i.show) : cardHTML(i, { showTitle:false })).join("");

      if (totalPages > 1 || list.length > PAGE_SIZE) {
        pager.innerHTML = `
          <button ${currentPage<=1?"disabled":""} data-action="goto-page" data-page="${currentPage-1}" aria-label="Previous page">‹ Prev</button>
          <span class="pageinfo">Page ${currentPage} of ${totalPages} — ${list.length.toLocaleString()} ${selectedFacetValues.size ? "filtered " : ""}titles</span>
          <button ${currentPage>=totalPages?"disabled":""} data-action="goto-page" data-page="${currentPage+1}" aria-label="Next page">Next ›</button>`;
      } else {
        pager.innerHTML = `<span class="pageinfo">${list.length.toLocaleString()} ${selectedFacetValues.size ? "filtered " : ""}title${list.length===1?"":"s"}</span>`;
      }
    }

    let storageData = null;
    let storageLoading = false;
    let storageScanStatus = "";
    async function renderStorage() {
      const panel = $("#storagePanel");
      if (!storageData && !storageLoading) {
        storageLoading = true;
        panel.innerHTML = '<div class="empty-state">Reading indexed storage…</div>';
        try {
          const response = await fetch(api("/api/library/storage"));
          if (!response.ok) throw new Error("Storage summary unavailable");
          storageData = await response.json();
        } catch {
          panel.innerHTML = '<div class="empty-state">Could not load storage details.</div>';
          storageLoading = false;
          return;
        }
        storageLoading = false;
      }
      if (!storageData) return;
      const data = storageData;
      const fmt = n => {
        n = Number(n) || 0;
        if (n < 1024) return `${n} B`;
        const units = ["KB","MB","GB","TB","PB"];
        let i = -1; do { n /= 1024; i++; } while (n >= 1024 && i < units.length - 1);
        return `${n.toFixed(n >= 100 ? 0 : n >= 10 ? 1 : 2)} ${units[i]}`;
      };
      const folderHTML = node => {
        const summary = `<span class="storage-name">📁 ${escapeHTML(node.name || "Library root")}</span>
          <span class="storage-meta">${Number(node.itemCount || 0).toLocaleString()} files · ${fmt(node.sizeBytes)}</span>`;
        const contents = [
          ...(node.children || []).map(folderHTML),
          ...(node.files || []).map(file => `<div class="storage-file"><span>▤ ${escapeHTML(file.name)}</span><span>${fmt(file.sizeBytes)}</span></div>`)
        ].join("");
        return `<details class="storage-folder"><summary>${summary}</summary><div class="storage-children">${contents || '<p class="storage-empty">No indexed files here.</p>'}</div></details>`;
      };
      panel.innerHTML = `
        <div class="storage-heading"><div><h2>Storage breakdown</h2><p>Indexed media only · Folder sizes come from the last scan</p><p id="storageScanStatus" role="status">${escapeHTML(storageScanStatus)}</p></div><button id="storageScanBtn" class="storage-scan" data-action="scan-storage">Scan now</button></div>
        <div class="storage-metrics">
          <div><strong>${fmt(data.totalBytes)}</strong><span>Indexed media</span></div>
          <div><strong>${Number(data.itemCount || 0).toLocaleString()}</strong><span>Files</span></div>
          <div><strong>${Number(data.folderCount || 0).toLocaleString()}</strong><span>Folders</span></div>
          <div><strong>${Number(data.showCount || 0).toLocaleString()}</strong><span>TV shows</span></div>
          <div><strong>${Number(data.artistCount || 0).toLocaleString()}</strong><span>Creators & studios</span></div>
        </div>
        <div class="storage-libraries">${(data.libraries || []).map(lib => `
          <section class="storage-library"><div class="storage-library-head">
            <strong>${escapeHTML(lib.name)}</strong><span>${escapeHTML(lib.kind)} · ${fmt(lib.root.sizeBytes)} · ${Number(lib.root.itemCount || 0).toLocaleString()} files</span>
          </div>${folderHTML(lib.root)}</section>`).join("") || '<div class="empty-state">No libraries are configured.</div>'}
        </div>`;
    }

    function gotoPage(n) { currentPage = n; render(); syncHash(true); window.scrollTo({ top:0 }); }

    function setStorageScanStatus(message, scanning = false) {
      storageScanStatus = message;
      const status = $("#storageScanStatus");
      if (status) status.textContent = message;
      const button = $("#storageScanBtn");
      if (button) {
        button.disabled = scanning;
        button.textContent = scanning ? "Scanning…" : "Scan now";
      }
    }

    async function scanStorageNow() {
      setStorageScanStatus("Starting scan…", true);
      try {
        const response = await fetch(api("/api/settings/rescan"), { method: "POST" });
        if (response.status === 202 || response.status === 409) {
          setStorageScanStatus(response.status === 409 ? "A scan is already running…" : "Scanning your libraries…", true);
          pollStorageScan(0, false);
          return;
        }
        const error = await response.json().catch(() => ({}));
        setStorageScanStatus(error.error || "Could not start the scan.");
      } catch {
        setStorageScanStatus("Could not reach the server to start the scan.");
      }
    }

    function pollStorageScan(attempt, sawScan) {
      setTimeout(async () => {
        try {
          const response = await fetch(api("/api/status"));
          if (!response.ok) throw new Error("Status unavailable");
          const state = await response.json();
          const active = !!state.scanning;
          sawScan = sawScan || active;
          if (active) {
            setStorageScanStatus(`Scanning… ${Number(state.itemsSeen || 0).toLocaleString()} files checked`, true);
            pollStorageScan(attempt + 1, sawScan);
            return;
          }
          // Allow the background scan goroutine to start after the 202 response.
          if (!sawScan && attempt < 3) {
            pollStorageScan(attempt + 1, false);
            return;
          }
          await refreshLibrary();
          const result = state.lastResult || {};
          setStorageScanStatus(`Scan complete · ${Number(state.itemCount || 0).toLocaleString()} items · ${Number(result.added || 0)} added · ${Number(result.updated || 0)} updated · ${Number(result.removed || 0)} removed`);
        } catch {
          if (attempt < 300) pollStorageScan(attempt + 1, sawScan);
          else setStorageScanStatus("Scan status unavailable. Check the server dashboard.");
        }
      }, 1000);
    }

    // Delegated clicks for grid/episode/edition rows (no inline JS strings).
    document.addEventListener("input", e => {
      const input = e.target.closest("[data-list-book-search]");
      if (input) updateListBookChoices(input);
    });
    document.addEventListener("click", e => {
      const btn = e.target.closest("[data-action]");
      if (!btn) return;
      const act = btn.dataset.action;
      if (act === "scan-storage") scanStorageNow();
      else if (act === "toggle-movie-detail") toggleMovieDetail();
      else if (act === "play-item" && btn.dataset.id) startPlaybackById(btn.dataset.id);
      else if (act === "expand-movie-shelf") { movieShelfExpanded = true; renderMovieCatalog(visibleItems()); }
      else if (act === "collapse-movie-shelf") { movieShelfExpanded = false; renderMovieCatalog(visibleItems()); }
      else if (act === "expand-library-shelf") { libraryShelfLimit += 96; render(); }
      else if (act === "collapse-library-shelf") { libraryShelfLimit = 96; render(); }
      else if (act === "browse-tab" && btn.dataset.tab) {
        const target = [...document.querySelectorAll("#tabs button[data-tab]")].find(tab => tab.dataset.tab === btn.dataset.tab);
        target?.click();
      }
      else if (act === "open-detail" && btn.dataset.id) openDetail(btn.dataset.id);
      else if (act === "open-show" && btn.dataset.show) openShowPage(btn.dataset.show);
      else if (act === "open-season") { openSeason = Number(btn.dataset.season); render(); syncHash(true); window.scrollTo({top:0}); }
      else if (act === "switch-copy" && btn.dataset.id) switchCopy(btn.dataset.id);
      else if (act === "goto-page") { gotoPage(parseInt(btn.dataset.page, 10)); }
      else if (act === "browse" && btn.dataset.path !== undefined) browseTo(btn.dataset.path);
      else if (act === "filter-facet" && btn.dataset.facetKey && btn.dataset.facetValue) {
        applyFacetFilter(btn.dataset.facetKey, btn.dataset.facetValue);
      }
      else if (act === "use-recommended-list" && btn.dataset.recommendedId) {
        useRecommendedList(btn.dataset.recommendedId);
      }
      else if (act === "export-missing" && btn.dataset.listId) {
        exportMissingList(btn.dataset.listId);
      }
      else if (act === "open-reading-list" && btn.dataset.listId) {
        selectedListID = btn.dataset.listId;
        syncHash(true);
        renderLists();
        window.scrollTo({ top: 0, behavior: "smooth" });
      }
      else if (act === "back-to-lists") {
        selectedListID = null;
        syncHash(true);
        renderLists();
        window.scrollTo({ top: 0, behavior: "smooth" });
      }
      else if (act === "add-list-item") {
        const card = btn.closest("[data-list-id]");
        const itemID = card?.querySelector("[data-list-select]")?.value;
        const tags = (card?.querySelector("[data-list-tags]")?.value || "").split(",").map(value => value.trim()).filter(Boolean);
        if (!itemID) return;
        listMutation(`/api/lists/${encodeURIComponent(btn.dataset.listId)}/items`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ itemID, position: -1, tags }) }).catch(error => alert(error.message || "Could not add book"));
      }
      else if (act === "remove-list-item") {
        listMutation(`/api/lists/${encodeURIComponent(btn.dataset.listId)}/items/${encodeURIComponent(btn.dataset.itemId)}`, { method: "DELETE" }).catch(error => alert(error.message || "Could not remove book"));
      }
      else if (act === "delete-list") {
        if (confirm("Delete this reading list? The catalog books will remain.")) listMutation(`/api/lists/${encodeURIComponent(btn.dataset.listId)}`, { method: "DELETE" }).catch(error => alert(error.message || "Could not delete list"));
      }
      else if (act === "move-list-item") {
        const list = lists.find(candidate => candidate.id === btn.dataset.listId);
        if (!list) return;
        const order = (list.itemIDs || []).slice();
        const index = Number(btn.dataset.index);
        const next = btn.dataset.direction === "up" ? index - 1 : index + 1;
        if (index < 0 || next < 0 || next >= order.length) return;
        [order[index], order[next]] = [order[next], order[index]];
        listMutation(`/api/lists/${encodeURIComponent(list.id)}/reorder`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ itemIDs: order }) }).catch(error => alert(error.message || "Could not reorder list"));
      }
    });

    function editionLabel(c) {
      // Lead with quality: resolution (probed or from the filename), then
      // container, runtime, and edition tag.
      const h = copyHeight(c);
      const quality = h ? (h >= 2160 ? "4K" : h >= 720 ? h + "p" : h + "p") : "";
      const bits = [quality,
        c.format ? "." + String(c.format).toUpperCase() : ""];
      if (c.durationSeconds) bits.push(formatTime(c.durationSeconds));
      if (c.edition) bits.push(String(c.edition));
      return bits.filter(Boolean).join(" · ");
    }
    function editionsHTML(item) {
      const copies = copiesOf(item);
      if (copies.length <= 1) return "";
      return `
      <div class="editions">
        <h4>${copies.length} versions — stream in</h4>
        ${copies.map(c => {
          const cp = progressFor(c.id);
          const cw = isWatched(cp, c);
          const current = c.id === item.id;
          return `
          <button class="ed-row ${current ? "current" : ""}" data-id="${c.id}" data-action="switch-copy"
                  aria-label="Stream copy ${editionLabel(c)}">
            <span class="edwatch">${cw ? "WATCHED" : (cp && cp.seconds > 5 ? "RESUME" : "")}</span>
            <span class="edlabel">${current ? "▶ " : ""}${escapeHTML(editionLabel(c))}</span>
            <span class="edmeta">${current ? "playing" : "switch"}</span>
          </button>`;
        }).join("")}
      </div>`;
    }

    function switchCopy(id) {
      if (activeTab === "movies" && libraryLayout === "rails" && !$("#movieCatalog")?.hidden) {
        selectedMovieID = id;
        renderMovieCatalog(visibleItems());
        return;
      }
      const wasOpen = $("#detail").open;
      openDetail(id, wasOpen);
    }

    function applyFacetFilter(key, value) {
      activeFacet = key;
      selectedFacetValues.clear();
      selectedFacetValues.add(String(value).toLowerCase());
      facetQuery = "";
      chipsExpanded = false;
      genreCategory = null;
      const search = $("#facetSearch");
      if (search) search.value = "";
      closeDetail();
      renderFacets();
      render();
      window.scrollTo({ top: 0, behavior: "smooth" });
    }

    function openDetail(id, keepVideo) {
      const item = items.find(i => i.id === id);
      if (!item) return;
      if (activeTab === "movies" && libraryLayout === "rails" && !$("#movieCatalog")?.hidden) {
        openMovieDetail(id);
        return;
      }
      const p = progressFor(id);
      const plan = playbackPlan(item);
      const resumeAt = p && p.seconds > 5 ? p.seconds : 0;
      const isCurrent = nowPlayingItem && nowPlayingItem.id === item.id;
      const media = npMedia();
      const playingNow = isCurrent && media && !media.paused;

      // Playback lives in the persistent controller so it survives closing
      // this modal; here we only show metadata and a transport button.
      const playerHTML = plan
        ? `<div class="play-hint">${mediaLabel(plan.mode)} plays in the player at the bottom and keeps playing while you browse.</div>`
        : `<div class="not-playable"><strong>.${escapeHTML((item.format || "?").toUpperCase())}</strong> can't play here.
             <a href="${api("/stream/" + item.id)}" target="_blank" rel="noopener">Open in a native player</a>.</div>`;

      $("#detailTitle").textContent =
        item.showTitle && item.seasonNumber != null
          ? `${item.showTitle} — S${String(item.seasonNumber).padStart(2,"0")}E${String(item.episodeNumber ?? 0).padStart(2,"0")} · ${item.title}`
          : item.title;

      const tags = (item.tags || []).slice(0, 10).map(tg => `<span class="tag">${escapeHTML(tg)}</span>`).join("");
      const detailFacets = [];
      if (item.author) detailFacets.push(`<button type="button" class="detail-link" data-action="filter-facet" data-facet-key="authors" data-facet-value="${escapeHTML(item.author)}">Author: ${escapeHTML(item.author)}</button>`);
      if (item.narrator) detailFacets.push(`<button type="button" class="detail-link" data-action="filter-facet" data-facet-key="narrators" data-facet-value="${escapeHTML(item.narrator)}">Narrator: ${escapeHTML(item.narrator)}</button>`);
      if (item.series) detailFacets.push(`<button type="button" class="detail-link" data-action="filter-facet" data-facet-key="series" data-facet-value="${escapeHTML(item.series)}">Series: ${escapeHTML(item.series)}</button>`);
      for (const genre of (Array.isArray(item.genres) ? item.genres : []).slice(0, 8)) {
        if (genre) detailFacets.push(`<button type="button" class="detail-link" data-action="filter-facet" data-facet-key="genres" data-facet-value="${escapeHTML(genre)}">Genre: ${escapeHTML(genre)}</button>`);
      }
      const detailFacetHTML = detailFacets.length ? `<div class="detail-facets" aria-label="Related audiobook filters">${detailFacets.join("")}</div>` : "";
      const summaryBits = [item.summary, item.subtitle].filter(Boolean)
        .map(s => `<p class="summary">${escapeHTML(s)}</p>`).join("");
      const playLabel = playingNow ? "Pause" : resumeAt ? "Resume at " + formatTime(resumeAt) : "Play";

      $("#detailBody").innerHTML = `
        ${playerHTML}
        <div class="meta-line">${[kindLabel(item.kind), item.year || "", runtimeLabel(item),
          (item.probedHeight ? item.probedWidth + "×" + item.probedHeight : "")]
          .filter(Boolean).join(" • ")}</div>
        ${summaryBits}
        ${detailFacetHTML}
        ${tags ? `<div class="tagrow">${tags}</div>` : ""}
        ${editionsHTML(item)}
        <div class="actions">
          ${plan ? `<button class="primary" onclick="startPlaybackById('${escapeHTML(item.id)}')">${playLabel}</button>` : ""}
          <a href="${api("/stream/" + item.id)}" target="_blank" rel="noopener">Open stream URL</a>
        </div>`;

      const dlg = $("#detail");
      if (!dlg.open) dlg.showModal();
    }

    function closeDetail() {
      // Playback intentionally continues in the persistent controller.
      $("#detail").close();
      render(); // refresh progress badges/bars
    }
    $("#detail").addEventListener("click", e => { if (e.target === $("#detail")) closeDetail(); });
    document.addEventListener("keydown", e => { if (e.key === "Escape") closeDetail(); });

    // Search input is debounced; selects fire immediately.
    let searchDebounce = null;
    for (const [id, ev] of [["q","input"],["watched","change"],["sort","change"]]) {
      $(`#${id}`).addEventListener(ev, () => {
        if (ev === "input") {
          clearTimeout(searchDebounce);
          searchDebounce = setTimeout(() => { currentPage = 1; render(); }, 200);
        } else {
          currentPage = 1; render();
        }
      });
    }

    fetch(api("/api/library")).then(async response => {
      const etag = response.headers.get("ETag") || "";
      return { etag, data: await response.json() };
    }).then(({ etag, data }) => {
      applyLibraryLayout(data.serverSettings?.libraryLayout || "rails");
      hideEmptyLibraries = data.serverSettings?.hideEmptyLibraries !== false;
      applyTheme(data.theme?.preset || "earthy");
      items = data.items ?? [];
      (data.progress ?? []).forEach(pr => progressByID.set(pr.itemID, pr));
      if (!restoreCopyGroups(etag)) {
        rebuildCopyGroups();
        cacheCopyGroups(etag);
      }
      applyEmptyLibraryTabs();
      renderFacets();
      applyHash(); // restore tab/show/page from the URL on load
      render();
      refreshLists().catch(() => { lists = []; });
      if (activeTab === "optimize") {
        refreshOptimizationQueue();
        refreshAudiobookJobs();
      }
    }).catch(() => {
      $("#grid").innerHTML = `<div class="empty-state">Could not load the library. Is the token valid?</div>`;
    });

    // ---------- Settings ----------
    let settingsData = null;
    let cacheStatusData = null;
    let networkStatusData = null;
    let networkExposureApplying = false;
    document.querySelector("#settingsBtn").addEventListener("click", openSettings);
    document.querySelector("#applyNetworkExposureBtn").addEventListener("click", applyNetworkExposure);
    on("#networkMode", "change", event => {
      if (event.target.value !== "interface") document.querySelector("#networkInterface").value = "";
    });
    on("#themeSel", "change", event => applyTheme(event.target.value));
    on("#libraryLayoutSel", "change", event => {
      applyLibraryLayout(event.target.value);
      render();
    });
    on("#hideEmptyLibraries", "change", event => {
      hideEmptyLibraries = event.target.checked;
      applyEmptyLibraryTabs();
      render();
    });
    on("#dataImportBtn", "click", () => $("#dataImportFile")?.click());
    on("#dataImportFile", "change", async e => {
      const file = e.target.files?.[0];
      const status = $("#dataPortabilityStatus");
      if (!file) return;
      if (!confirm("Import this Sonder data bundle and replace the current catalog data? Media files will not be changed and no scan will start.")) {
        e.target.value = "";
        return;
      }
      status.textContent = "Importing…";
      try {
        const response = await fetch(api("/api/data/import"), {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: await file.text(),
        });
        const result = await response.json().catch(() => ({}));
        if (!response.ok) throw new Error(result.error || "Import failed");
        await refreshLibrary();
        await refreshLists();
        status.textContent = `Imported ${Number(result.imported?.items || 0).toLocaleString()} items · no scan started`;
      } catch (error) {
        status.textContent = error.message || "Import failed";
      } finally {
        e.target.value = "";
      }
    });
    document.querySelector("#addLibDetails").addEventListener("toggle", e => {
      if (e.target.open) browseTo("");
    });

    function openSettings() {
      const dlg = document.querySelector("#settingsDlg");
      dlg.showModal();
      loadSettings();
      loadNetworkExposure();
    }

    async function loadSettings() {
      const status = document.querySelector("#settingsStatus");
      status.textContent = "Loading…";
      try {
        settingsData = await (await fetch(api("/api/settings"))).json();
        await loadCacheStatus();
        applyLibraryLayout(settingsData.libraryLayout || "rails");
        hideEmptyLibraries = settingsData.hideEmptyLibraries !== false;
        applyEmptyLibraryTabs();
        applyTheme(settingsData.themePreset || "earthy");
        pendingLibs = null; // fresh server state wins over stale edits
      } catch { settingsData = null; }
      renderSettings();
      status.textContent = "";
    }

    async function loadCacheStatus() {
      try {
        const response = await fetch(api("/api/cache/status"), { cache: "no-store" });
        cacheStatusData = response.ok ? await response.json() : null;
      } catch (_) {
        cacheStatusData = null;
      }
    }

    function renderSettings() {
      if (!settingsData) return;
      const rows = document.querySelector("#libRows");
      rows.innerHTML = (settingsData.libraries ?? []).map((l, idx) => `
        <div style="display:grid;grid-template-columns:1fr auto;gap:8px;align-items:center;padding:8px 0;border-bottom:1px solid var(--line)">
          <div>
            <strong>${escapeHTML(l.name)}</strong>
            <span style="color:var(--muted)">· ${l.itemCount} items · ${escapeHTML(l.kind)}</span>
            <p style="margin:2px 0 0;font-size:12px">${escapeHTML(l.path)}</p>
          </div>
          <button onclick="removeLibraryRow(${idx})" aria-label="Remove ${escapeHTML(l.name)}">Remove</button>
        </div>`).join("") || `<p style="color:var(--muted)">No libraries configured.</p>`;

      document.querySelector("#allowLAN").checked = !!settingsData.allowLAN;
      document.querySelector("#themeSel").value = settingsData.themePreset || "earthy";
      document.querySelector("#libraryLayoutSel").value = settingsData.libraryLayout || "rails";
      document.querySelector("#audiobookLayoutSel").value = settingsData.audiobookLayout || "rails";
      document.querySelector("#hideEmptyLibraries").checked = settingsData.hideEmptyLibraries !== false;
      const cache = settingsData.mediaCache || {};
      document.querySelector("#mediaCacheEnabled").checked = cache.enabled !== false;
      document.querySelector("#mediaCacheMaxGiB").value = (Number(cache.maxBytes || 0) / (1024 ** 3)).toFixed(2);
      document.querySelector("#mediaCacheMinFreeGiB").value = (Number(cache.minFreeBytes || 0) / (1024 ** 3)).toFixed(2);
      const cacheStatus = document.querySelector("#mediaCacheStatus");
      if (cacheStatus) {
        const live = cacheStatusData || {};
        const state = live.enabled === false ? "disabled" : "enabled";
        const cached = formatOptimizationBytes(live.cachedBytes || 0);
        const limit = formatOptimizationBytes(live.maxBytes || cache.maxBytes || 0);
        const free = formatOptimizationBytes(live.freeBytes || 0);
        cacheStatus.textContent = `Currently ${state} · ${cached} cached of ${limit} · ${free} free`;
      }
      document.querySelector("#mounts").innerHTML =
        (settingsData.suggestedMounts ?? []).map(m => `<option value="${escapeHTML(m)}">`).join("");

      const line = document.querySelector("#pairingLine");
      if (settingsData.pairingToken) {
        line.innerHTML = `Pairing token: <code id="tok" style="user-select:all">${settingsData.pairingToken}</code>`;
      } else if (settingsData.tokenConfigured) {
        line.innerHTML = `Pairing token is configured (visible only from this Mac).`;
      } else {
        line.textContent = "No pairing token (LAN access is open).";
      }
    }

    async function loadNetworkExposure() {
      const status = document.querySelector("#networkExposureStatus");
      status.textContent = "Checking listeners…";
      try {
        const response = await fetch(api("/api/network/status"), { cache: "no-store" });
        const payload = await response.json();
        if (!response.ok) throw new Error(payload.error || "Network status unavailable");
        networkStatusData = payload.status;
        renderNetworkExposure(networkStatusData);
      } catch (error) {
        status.textContent = error.message || "Network status unavailable";
      }
    }

    function renderNetworkExposure(status) {
      if (!status?.state) return;
      const mode = document.querySelector("#networkMode");
      const iface = document.querySelector("#networkInterface");
      const webPort = document.querySelector("#networkWebPort");
      const apiPort = document.querySelector("#networkAPIPort");
      const line = document.querySelector("#networkExposureStatus");
      const apply = document.querySelector("#applyNetworkExposureBtn");
      const interfaces = status.interfaces ?? [];
      const options = [`<option value="">Automatic for selected mode</option>`].concat(
        interfaces.filter(item => item.active).map(item =>
          `<option value="${escapeHTML(item.id)}">${escapeHTML(item.id)} · ${escapeHTML(item.ipv4)}</option>`)
      );
      iface.innerHTML = options.join("");
      mode.value = status.state.selectedMode || "loopback";
      iface.value = status.state.selectedInterface || "";
      webPort.value = status.state.webPort || "";
      apiPort.value = status.state.apiPort || "";
      const caddy = status.caddy?.enabled ? `Caddy ${status.caddy.listening ? "listening" : "down"}` : "Caddy off";
      line.textContent = networkExposureApplying || status.rebinding
        ? "Applying exposure… reconnecting listeners"
        : `Web ${status.state.webBindAddress}:${status.state.webPort} ${status.webHealthy ? "up" : "down"} · API 127.0.0.1:${status.state.apiPort} ${status.apiHealthy ? "ready" : "down"} · ${caddy}`;
      apply.disabled = networkExposureApplying || status.rebinding;
    }

    function exposureStatusURL(target) {
      const state = target?.state || target || {};
      const u = new URL("/api/network/status", window.location.href);
      let host = state.webBindAddress || state.selectedIPv4 || window.location.hostname;
      if (host === "0.0.0.0" || host === "::" || !host) host = window.location.hostname;
      u.hostname = host;
      if (state.webPort) u.port = String(state.webPort);
      if (Sonder.TOKEN) u.searchParams.set("token", Sonder.TOKEN);
      return u.toString();
    }

    function pollNetworkExposure(target, attempt = 0) {
      if (attempt > 120) {
        networkExposureApplying = false;
        document.querySelector("#networkExposureStatus").textContent = "Timed out waiting for listeners";
        loadNetworkExposure();
        return;
      }
      setTimeout(async () => {
        try {
          const response = await fetch(exposureStatusURL(target), { cache: "no-store" });
          const payload = await response.json();
          if (!response.ok) throw new Error(payload.error || "Network status unavailable");
          const status = payload.status;
          networkStatusData = status;
          renderNetworkExposure(status);
          const expected = target?.state || target || {};
          const settled = !status.rebinding && status.webHealthy && status.apiHealthy &&
            (!expected.webPort || status.state.webPort === expected.webPort) &&
            (!expected.apiPort || status.state.apiPort === expected.apiPort);
          if (!settled) return pollNetworkExposure(target, attempt + 1);
          networkExposureApplying = false;
          renderNetworkExposure(status);
          const currentPort = Number(window.location.port || (window.location.protocol === "https:" ? 443 : 80));
          if (expected.webPort && currentPort !== expected.webPort) {
            document.querySelector("#networkExposureStatus").textContent = `Live on port ${expected.webPort}; reopening…`;
            window.location.assign(exposureStatusURL(expected).replace("/api/network/status", "/"));
          } else {
            document.querySelector("#networkExposureStatus").textContent = "Exposure updated ✓";
          }
        } catch (_) {
          pollNetworkExposure(target, attempt + 1);
        }
      }, 500);
    }

    async function applyNetworkExposure() {
      const status = document.querySelector("#networkExposureStatus");
      const body = {
        mode: document.querySelector("#networkMode").value,
        interfaceID: document.querySelector("#networkInterface").value,
        webPort: Number(document.querySelector("#networkWebPort").value),
        apiPort: Number(document.querySelector("#networkAPIPort").value),
      };
      if (!Number.isInteger(body.webPort) || !Number.isInteger(body.apiPort) || body.webPort < 1 || body.webPort > 65535 || body.apiPort < 1 || body.apiPort > 65535) {
        status.textContent = "Ports must be between 1 and 65535";
        return;
      }
      if (body.webPort === body.apiPort) {
        status.textContent = "Web and API ports must be different";
        return;
      }
      networkExposureApplying = true;
      renderNetworkExposure(networkStatusData);
      status.textContent = "Validating exposure…";
      try {
        const response = await fetch(api("/api/network/exposure"), {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(body),
        });
        const payload = await response.json().catch(() => ({}));
        if (!response.ok) throw new Error(payload.error || "Exposure change rejected");
        status.textContent = "Applying exposure…";
        pollNetworkExposure(payload.target, 0);
      } catch (error) {
        networkExposureApplying = false;
        status.textContent = error.message || "Exposure change failed";
        renderNetworkExposure(networkStatusData);
      }
    }

    let pendingLibs = null;
    let browsedPath = "";
    function currentLibraries() {
      return pendingLibs ?? (settingsData?.libraries ?? []).map(l => ({
        id: l.id, name: l.name, path: l.path, kind: l.kind,
      }));
    }
    function addLibraryRow() {
      const name = document.querySelector("#newName").value.trim();
      const path = document.querySelector("#selectedPath").textContent.trim() ||
                   browsedPath;
      const kind = document.querySelector("#newKind").value;
      if (!path) return flashStatus("Pick a folder first");
      pendingLibs = currentLibraries().concat([{ id:"", name, path, kind }]);
      document.querySelector("#newName").value = "";
      document.querySelector("#selectedPath").textContent = "";
      browsedPath = "";
      applyPendingLibs();
      flashStatus("Library added — Save changes to scan it");
    }
    function removeLibraryRow(idx) {
      pendingLibs = currentLibraries().filter((_, i) => i !== idx);
      applyPendingLibs();
    }
    function applyPendingLibs() {
      const counts = new Map((settingsData?.libraries ?? []).map(l => [l.path, l.itemCount]));
      settingsData.libraries = currentLibraries().map(l => ({ ...l, itemCount: counts.get(l.path) ?? 0 }));
      renderSettings();
      pendingLibs = currentLibraries();
    }

    async function saveSettings() {
      const body = {};
      if (pendingLibs) body.libraries = currentLibraries().map(({ id, name, path, kind }) => ({ id, name, path, kind }));
      body.allowLAN = document.querySelector("#allowLAN").checked;
      body.themePreset = document.querySelector("#themeSel").value;
      body.libraryLayout = document.querySelector("#libraryLayoutSel").value;
      body.audiobookLayout = document.querySelector("#audiobookLayoutSel").value;
      body.hideEmptyLibraries = document.querySelector("#hideEmptyLibraries").checked;
      const maxGiB = Number(document.querySelector("#mediaCacheMaxGiB").value);
      const minFreeGiB = Number(document.querySelector("#mediaCacheMinFreeGiB").value);
      if (!Number.isFinite(maxGiB) || maxGiB <= 0 || !Number.isFinite(minFreeGiB) || minFreeGiB < 0) {
        flashStatus("Cache limit must be greater than 0 and free-space reserve cannot be negative");
        return;
      }
      body.mediaCache = {
        enabled: document.querySelector("#mediaCacheEnabled").checked,
        maxBytes: Math.round(maxGiB * (1024 ** 3)),
        minFreeBytes: Math.round(minFreeGiB * (1024 ** 3)),
      };
      // Keep the older movie/TV keys synchronized with the shared browser
      // layout while the audiobook player keeps its own explicit preference.
      body.moviesLayout = body.libraryLayout === "classic" ? "grid" : "rails";
      body.tvLayout = body.moviesLayout;
      await putSettings(body);
      pendingLibs = null;
      // library table may have changed -> refresh catalog behind the dialog
      fetch(api("/api/library")).then(async response => {
        const etag = response.headers.get("ETag") || "";
        return { etag, data: await response.json() };
      }).then(({ etag, data }) => {
        items = data.items ?? [];
        (data.progress ?? []).forEach(pr => progressByID.set(pr.itemID, pr));
        storageData = null;
        if (!restoreCopyGroups(etag)) {
          rebuildCopyGroups();
          cacheCopyGroups(etag);
        }
        renderSettings();
        render();
      });
    }

    async function rescanNow() {
      const r = await fetch(api("/api/settings/rescan"), { method:"POST" });
      flashStatus(r.status === 202 ? "Rescan started…" : "Scan already running");
      if (r.status === 202) pollUntilScanned();
    }

    // Poll /api/status until the background job (scan or enrichment) finishes.
    function pollStatus(done, attempt = 0) {
      if (attempt > 300) return flashStatus("Still working — check back later");
      setTimeout(() => {
        fetch(api("/api/status")).then(r => r.json()).then(st => {
          if (st.scanning || st.enriching) return pollStatus(done, attempt + 1);
          done();
        }).catch(() => pollStatus(done, attempt + 1));
      }, 2000);
    }
    async function refreshLibrary() {
      const response = await fetch(api("/api/library"));
      const etag = response.headers.get("ETag") || "";
      const data = await response.json();
      items = data.items ?? [];
      hideEmptyLibraries = data.serverSettings?.hideEmptyLibraries !== false;
      applyEmptyLibraryTabs();
      storageData = null;
      progressByID.clear();
      (data.progress ?? []).forEach(pr => progressByID.set(pr.itemID, pr));
      if (!restoreCopyGroups(etag)) {
        rebuildCopyGroups();
        cacheCopyGroups(etag);
      }
      render();
      refreshLibraryHealth();
    }

    async function refreshLibraryHealth() {
      const panel = document.querySelector("#libraryHealth");
      try {
        const response = await fetch(api("/api/library/health"));
        if (!response.ok) throw new Error("Audit unavailable");
        const report = await response.json();
        document.querySelector("#libraryHealthSummary").textContent =
          `Library consistency: ${report.issues.length} warnings · ${report.browsingGroupCount ?? report.showCount} show groups / ${report.representedFolders} represented folders (${report.showCount} parsed names)`;
        document.querySelector("#libraryHealthIssues").innerHTML = report.issues.slice(0,100).map(issue =>
          `<p><strong>${escapeHTML(issue.folder || "TV library")}</strong>: ${escapeHTML(issue.message)} ${escapeHTML(issue.shows.join(" · "))}</p>`).join("") +
          `<p>${report.unmappedItems} items could not be mapped to a show folder.${report.issues.length > 100 ? " Showing the first 100 warnings." : ""}</p>`;
        panel.hidden = false;
      } catch {
        panel.hidden = false;
        document.querySelector("#libraryHealthSummary").textContent = "Library consistency check unavailable";
      }
    }
    const scheduleIdle = (task, timeout = 1500) => {
      if (typeof requestIdleCallback === "function") requestIdleCallback(task, { timeout });
      else setTimeout(task, 0);
    };
    scheduleIdle(refreshLibraryHealth);

    const optimizationPanel = document.querySelector("#optimizationPage");
    let optimizationRefreshInFlight = false;
    let optimizationJobsRefreshInFlight = false;
    let optimizationQueuePostInFlight = false;
    let optimizationPollTimer = null;
    let optimizationCandidates = { audio: [], video: [], reviews: "" };
    let optimizationVisibleLimit = 40;
    let optimizationSearchTerm = "";
    const selectedOptimizationItems = new Set();
    const approvedOptimizationCovers = new Set();
    const optimizationBitrates = new Map();
    const queuedOptimizationIDs = new Set();
    optimizationPanel.addEventListener("click", event => {
      const button = event.target.closest("button");
      if (!button) return;
      if (button.id === "optimizationRefresh") {
        refreshOptimizationQueue(); refreshAudiobookJobs();
      } else if (button.id === "optimizationQueueSelected") {
        queueSelectedAudiobooks();
      } else if (button.id === "optimizationPause") {
        controlAudiobookQueue();
      } else if (button.matches("[data-optimization-show-more]")) {
        optimizationVisibleLimit += 40;
        renderOptimizationCards();
      } else if (button.matches("[data-optimization-prioritize]")) {
        prioritizeAudiobookJob(button.dataset.optimizationPrioritize);
      } else if (button.matches("[data-optimization-promote]")) {
        promoteAudiobookJob(button.dataset.optimizationPromote);
      } else if (button.matches("[data-optimization-cancel]")) {
        cancelAudiobookJob(button.dataset.optimizationCancel);
      } else if (button.matches("[data-optimization-retry]")) {
        retryAudiobookJob(button.dataset.optimizationRetry);
      } else if (button.matches("[data-optimization-review]")) {
        reviewAudiobookJob(button.dataset.optimizationReview, button.dataset.optimizationDecision);
      }
    });
    optimizationPanel.addEventListener("change", event => {
      const select = event.target.closest("[data-optimization-bitrate]");
      if (select) {
        optimizationBitrates.set(select.dataset.optimizationBitrate, Number(select.value));
        updateOptimizationEstimate(select);
      }
      if (event.target.matches("[data-optimization-select]")) {
        if (event.target.checked) selectedOptimizationItems.add(event.target.dataset.optimizationSelect);
        else selectedOptimizationItems.delete(event.target.dataset.optimizationSelect);
        syncOptimizationSelectionState();
      }
      if (event.target.matches("[data-optimization-cover]")) {
        if (event.target.checked) approvedOptimizationCovers.add(event.target.dataset.optimizationCover);
        else approvedOptimizationCovers.delete(event.target.dataset.optimizationCover);
      }
    });
    document.querySelector("#optimizationSearch").addEventListener("input", event => {
      optimizationSearchTerm = event.target.value.trim().toLowerCase();
      optimizationVisibleLimit = 40;
      renderOptimizationCards();
    });
    document.querySelector("#optimizationClearSearch").addEventListener("click", () => {
      const input = document.querySelector("#optimizationSearch");
      input.value = "";
      optimizationSearchTerm = "";
      optimizationVisibleLimit = 40;
      renderOptimizationCards();
      input.focus();
    });

    function syncOptimizationSelectionState() {
      const count = selectedOptimizationItems.size;
      document.querySelector("#optimizationSelectedCount").textContent = `${count} selected`;
      document.querySelector("#optimizationQueueSelected").disabled = selectedOptimizationItems.size === 0 || optimizationQueuePostInFlight;
    }

    function formatOptimizationBytes(value) {
      let bytes = Number(value) || 0;
      const units = ["B", "KiB", "MiB", "GiB", "TiB"];
      let unit = 0;
      while (bytes >= 1024 && unit < units.length - 1) {
        bytes /= 1024;
        unit++;
      }
      return `${bytes.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`;
    }

    function formatOptimizationDuration(seconds) {
      seconds = Math.max(0, Math.round(Number(seconds) || 0));
      const h = Math.floor(seconds / 3600);
      const m = Math.floor((seconds % 3600) / 60);
      return h ? `${h}h ${m}m` : `${m}m`;
    }

    function optimizationETA(job) {
      const progress = Number(job.progress) || 0;
      const started = Date.parse(job.startedAt || "");
      if (!started || progress < 1 || progress >= 100 || !["copying-source", "encoding", "validating", "copying-result"].includes(job.status)) return "";
      const elapsed = Math.max(1, (Date.now() - started) / 1000);
      const remaining = elapsed * (100 - progress) / progress;
      return ` · elapsed ${formatOptimizationDuration(elapsed)} · ETA ${formatOptimizationDuration(remaining)}`;
    }

    function renderOptimizationRunSummary(jobs) {
      const activeStatuses = ["copying-source", "encoding", "validating", "copying-result"];
      const completedStatuses = ["staged-for-review", "accepted", "needs-revision"];
      const attentionStatuses = ["failed", "interrupted"];
      const active = jobs.find(job => activeStatuses.includes(job.status));
      const queued = jobs.filter(job => job.status === "queued");
      const completed = jobs.filter(job => completedStatuses.includes(job.status));
      const attention = jobs.filter(job => attentionStatuses.includes(job.status));
      const title = document.querySelector("#optimizationRunTitle");
      const detail = document.querySelector("#optimizationRunDetail");
      const state = document.querySelector("#optimizationRunState");
      const progressWrap = document.querySelector("#optimizationRunProgress");
      const progressBar = document.querySelector("#optimizationRunProgressBar");
      const progressText = document.querySelector("#optimizationRunProgressText");
      const eta = document.querySelector("#optimizationRunETA");
      document.querySelector("#optimizationCompletedCount").textContent = completed.length.toLocaleString();
      document.querySelector("#optimizationActiveCount").textContent = active ? "1" : "0";
      document.querySelector("#optimizationQueuedCount").textContent = queued.length.toLocaleString();
      document.querySelector("#optimizationAttentionCount").textContent = attention.length.toLocaleString();
      if (active) {
        const progress = Math.max(0, Math.min(100, Number(active.progress) || 0));
        title.textContent = active.title;
        detail.textContent = `${active.phase || "Working"} · ${formatOptimizationBytes(active.currentBytes)} of ${formatOptimizationBytes(active.sourceBytes)} · book ${completed.length + 1} of ${jobs.length}`;
        state.textContent = active.status.replaceAll("-", " ");
        state.className = "optimization-state optimization-state-active";
        progressWrap.hidden = false;
        progressBar.value = progress;
        progressText.textContent = `${progress.toFixed(1)}% on this book`;
        const etaText = optimizationETA(active);
        eta.textContent = etaText ? etaText.replace(/^ · /, "") : "Calculating ETA…";
      } else if (queued.length) {
        title.textContent = "Waiting to start the next book";
        detail.textContent = `${queued.length.toLocaleString()} books are queued for sequential processing.`;
        state.textContent = "queued";
        state.className = "optimization-state";
        progressWrap.hidden = true;
        eta.textContent = "";
      } else if (completed.length) {
        title.textContent = "Run complete or paused";
        detail.textContent = `${completed.length.toLocaleString()} books have reached staged review.`;
        state.textContent = "idle";
        state.className = "optimization-state";
        progressWrap.hidden = true;
        eta.textContent = "";
      } else {
        title.textContent = "No conversion running";
        detail.textContent = "Queue a book to see its live progress here.";
        state.textContent = "idle";
        state.className = "optimization-state";
        progressWrap.hidden = true;
        eta.textContent = "";
      }
      const next = queued.slice(0, 5);
      document.querySelector("#optimizationNextJobs").innerHTML = next.length
        ? `<strong>Next in queue</strong><ol>${next.map(job => `<li>${escapeHTML(job.title)}</li>`).join("")}</ol>`
        : attention.length ? `<strong>Needs attention</strong><span>${attention.slice(0, 3).map(job => escapeHTML(job.title)).join(" · ")}</span>` : "";
    }

    function updateOptimizationEstimate(select) {
      const estimate = select.closest("article")?.querySelector("[data-optimization-estimate]");
      if (!estimate) return;
      const current = Number(estimate.dataset.currentBytes) || 0;
      const baseOutput = Number(estimate.dataset.baseOutputBytes) || 0;
      const baseRate = Number(estimate.dataset.baseRate) || 0;
      const rate = Number(select.value) || 0;
      const output = baseRate > 0 ? baseOutput * rate / baseRate : 0;
      const saved = Math.max(0, current - output);
      const percent = current > 0 ? saved * 100 / current : 0;
      estimate.textContent = `Rough bitrate-based estimate at ${rate} kb/s: save ${formatOptimizationBytes(saved)} (${percent.toFixed(1)}%) · full output will be staged for listening review`;
    }

    function renderOptimizationCards() {
      document.querySelector("#optimizationAudioCount").textContent = optimizationCandidates.audio.filter(card => !queuedOptimizationIDs.has(card.id)).length.toLocaleString();
      const matches = cards => cards.filter(card => !queuedOptimizationIDs.has(card.id) && (!optimizationSearchTerm || card.title.toLowerCase().includes(optimizationSearchTerm)));
      const renderList = (cards, kind) => {
        const filtered = matches(cards);
        const shown = filtered.slice(0, optimizationVisibleLimit);
        const more = filtered.length > shown.length
          ? `<button type="button" class="optimization-more" data-optimization-show-more="${kind}">Show 40 more · ${filtered.length - shown.length} remaining</button>` : "";
        const count = `<p class="optimization-result-count">Showing ${shown.length} of ${filtered.length.toLocaleString()}</p>`;
        return count + (shown.map(card => card.html).join("") || `<p class="optimization-empty">${optimizationSearchTerm ? "No recommendations match that title." : "No current recommendations."}</p>`) + more;
      };
      document.querySelector("#optimizationAudiobooks").innerHTML =
        `<p class="optimization-quality">${escapeHTML(optimizationCandidates.qualityNotice || "")}</p>` + renderList(optimizationCandidates.audio, "audio");
      document.querySelector("#optimizationVideoSamples").innerHTML = renderList(optimizationCandidates.video, "video");
      document.querySelector("#optimizationReviews").innerHTML = optimizationCandidates.reviews || `<p class="optimization-empty">No previous recommendations need review.</p>`;
      syncOptimizationSelectionState();
    }

    async function refreshOptimizationQueue() {
      const audioBody = document.querySelector("#optimizationAudiobooks");
      const videoBody = document.querySelector("#optimizationVideoSamples");
      const reviewBody = document.querySelector("#optimizationReviews");
      if (optimizationRefreshInFlight) return;
      optimizationRefreshInFlight = true;
      audioBody.setAttribute("aria-busy", "true");
      document.querySelector("#optimizationStatus").textContent = "Screening current catalog probes…";
      try {
        const response = await fetch(api("/api/optimization/queue"));
        if (!response.ok) throw new Error("Queue unavailable");
        const queue = await response.json();
        const jobs = (queue.jobs ?? []).map(job => {
          const estimate = job.estimatedSavingsBytes == null
            ? "Sample required; savings not estimated"
            : `<span data-optimization-estimate data-current-bytes="${Number(job.currentBytes)}" data-base-output-bytes="${Number(job.estimatedOutputBytes)}" data-base-rate="${Number(job.targetBitrateKbps)}">Rough bitrate-based estimate at ${Number(job.targetBitrateKbps)} kb/s: save ${formatOptimizationBytes(job.estimatedSavingsBytes)} (${Number(job.estimatedSavingsPct).toFixed(1)}%) · full output will be staged for listening review</span>`;
          const chosenRate = optimizationBitrates.get(job.id) || Number(job.targetBitrateKbps);
          const selection = job.family === "audiobook-opus"
            ? `<label class="optimization-select-label"><input type="checkbox" data-optimization-select="${escapeHTML(job.id)}"${selectedOptimizationItems.has(job.id) ? " checked" : ""}> Select</label>` +
              `<label>Opus trial bitrate <select data-optimization-bitrate="${escapeHTML(job.id)}">${(job.channels === 1 ? [24, 32, 40] : [48, 64]).map(rate => `<option value="${rate}"${rate === chosenRate ? " selected" : ""}>${rate} kb/s</option>`).join("")}</select></label>` +
              (!job.embeddedCoverPresent && job.catalogCoverAvailable
                ? `<label class="optimization-cover-approval"><input type="checkbox" data-optimization-cover="${escapeHTML(job.id)}"${approvedOptimizationCovers.has(job.id) ? " checked" : ""}> Use available catalog art as this output's cover</label>`
                : "")
            : "";
          const cover = job.family === "audiobook-opus"
            ? `<small>Embedded cover: ${job.embeddedCoverPresent ? "yes" : "no"} · catalog artwork: ${job.catalogCoverAvailable ? "available" : "not available"}</small><br>` : "";
          const html = `<article class="optimization-recommendation"><div class="optimization-rec-title"><strong>${escapeHTML(job.title)}</strong><span>${escapeHTML(job.sourceCodec)} → ${escapeHTML(job.targetCodec)}</span></div>` +
            `${formatOptimizationBytes(job.currentBytes)} · ${estimate}<br>` +
            `<p>${escapeHTML(job.reason)}</p>${cover}<small>${escapeHTML(job.playbackNote)}</small><div class="optimization-rec-controls">${selection}</div></article>`;
          return { id: job.id, family: job.family, title: job.title, html };
        });
        const reviews = (queue.reviews ?? []).map(review =>
          `<article class="optimization-recommendation"><strong>${escapeHTML(review.title)}</strong> · ${formatOptimizationBytes(review.currentBytes)}<p>${escapeHTML(review.reason)}</p></article>`
        ).join("");
        document.querySelector("#optimizationAudioCount").textContent = Number(queue.audioCandidates || 0).toLocaleString();
        document.querySelector("#optimizationVideoCount").textContent = Number(queue.videoSampleCandidates || 0).toLocaleString();
        document.querySelector("#optimizationReviewCount").textContent = Number(queue.reviewCount || 0).toLocaleString();
        document.querySelector("#optimizationSavings").textContent = formatOptimizationBytes(queue.estimatedAudioSavingsBytes);
        optimizationCandidates = {
          audio: jobs.filter(job => job.family === "audiobook-opus"),
          video: jobs.filter(job => job.family === "video-av1"),
          reviews,
          qualityNotice: queue.qualityNotice,
        };
        renderOptimizationCards();
        if (!optimizationPanel.hidden) document.querySelector("#optimizationStatus").textContent = "Analysis ready. Select audiobook trials, then start the queue.";
      } catch {
        for (const id of ["optimizationAudioCount", "optimizationVideoCount", "optimizationReviewCount", "optimizationSavings"])
          document.querySelector(`#${id}`).textContent = "—";
        audioBody.innerHTML = "<p role=\"alert\">Could not analyze the current catalog.</p>";
        videoBody.innerHTML = "<p>Video sample analysis unavailable.</p>";
        document.querySelector("#optimizationStatus").textContent = "Could not refresh recommendations.";
      } finally {
        audioBody.setAttribute("aria-busy", "false");
        optimizationRefreshInFlight = false;
      }
    }

    async function refreshAudiobookJobs() {
      if (optimizationPanel.hidden) return;
      if (optimizationJobsRefreshInFlight) return;
      optimizationJobsRefreshInFlight = true;
      const container = document.querySelector("#optimizationJobs");
      try {
        const response = await fetch(api("/api/optimization/audiobooks/jobs"));
        if (!response.ok) throw new Error("Job queue unavailable");
        const queue = await response.json();
        renderOptimizationRunSummary(queue.jobs ?? []);
        queuedOptimizationIDs.clear();
        for (const job of queue.jobs ?? []) {
          if (["queued", "copying-source", "encoding", "validating", "copying-result", "staged-for-review", "accepted", "needs-revision"].includes(job.status)) {
            queuedOptimizationIDs.add(job.itemID);
            selectedOptimizationItems.delete(job.itemID);
            approvedOptimizationCovers.delete(job.itemID);
          }
        }
        renderOptimizationCards();
        const pause = document.querySelector("#optimizationPause");
        const hasPendingJobs = (queue.jobs ?? []).some(job => ["queued", "copying-source", "encoding", "validating", "copying-result"].includes(job.status));
        pause.dataset.queueAction = queue.paused ? "resume" : "pause";
        pause.textContent = queue.paused ? "▶ Resume queue" : "Ⅱ Pause after this book";
        pause.disabled = !queue.paused && !hasPendingJobs;
        const jobs = queue.jobs ?? [];
        if (!jobs.length) {
          container.innerHTML = `<p class="optimization-empty">No audiobook jobs yet. Select trials above, then start the queue.</p>`;
        } else {
          container.innerHTML = jobs.map(job => {
            const cancellable = ["queued", "copying-source", "encoding", "validating", "copying-result"].includes(job.status);
            const retryable = ["failed", "interrupted", "canceled"].includes(job.status);
            const progress = Math.max(0, Math.min(100, Number(job.progress) || 0));
            const receipt = job.receipt ? `<p><strong>Verified reduction:</strong> ${formatOptimizationBytes(job.receipt.sourceBytes)} → ${formatOptimizationBytes(job.receipt.outputBytes)} · saved ${formatOptimizationBytes(job.receipt.savedBytes)} (${Number(job.receipt.savedPercent).toFixed(1)}%).<br>` +
              `SHA-256 checked on return · ${escapeHTML(job.receipt.validation.audioCodec)} in ${escapeHTML(job.receipt.validation.container)} · ${job.receipt.validation.chapterCount} chapters · cover, metadata and full decode verified.<br>` +
              `Cover: ${escapeHTML(job.receipt.coverSource)} · staged at <code>${escapeHTML(job.receipt.stagedRelativePath)}</code>.<br>` +
              `Listening/device review: ${escapeHTML(job.receipt.playbackReview)}.</p>` +
              `<audio controls preload="none" aria-label="Listen to staged Opus audiobook" src="${escapeHTML(api(`/api/optimization/audiobooks/jobs/${encodeURIComponent(job.id)}/stream`))}"></audio>` +
              (job.receipt.playbackReview === "pending human listening and target-device playback"
                ? `<div><button type="button" data-optimization-review="${escapeHTML(job.id)}" data-optimization-decision="accepted">Mark listening and playback passed</button>` +
                  `<button type="button" data-optimization-review="${escapeHTML(job.id)}" data-optimization-decision="needs-revision">Flag for revision</button></div>`
                : job.receipt.playbackReview?.startsWith("accepted")
                  ? `<div><button type="button" data-optimization-promote="${escapeHTML(job.id)}">Install Opus and remove original</button></div>` : "") : "";
            const poster = job.posterURL ? `<img class="optimization-job-poster" src="${escapeHTML(api(job.posterURL))}" alt="" loading="lazy">` : `<div class="optimization-job-poster optimization-job-poster-empty" aria-hidden="true">A</div>`;
            return `<article class="optimization-job"><div class="optimization-job-heading">${poster}<div><strong>${escapeHTML(job.title)}</strong><span class="optimization-state">${escapeHTML(job.status)}</span></div></div>` +
              `Opus ${Number(job.bitrateKbps)} kb/s · attempt ${Number(job.attempt)} · ${escapeHTML(job.phase)} · ${progress.toFixed(0)}%${optimizationETA(job)}<progress max="100" value="${progress}"></progress>` +
              (job.error ? `<p role="alert">${escapeHTML(job.error)}</p>` : "") + receipt +
              (job.status === "queued" ? `<button type="button" data-optimization-prioritize="${escapeHTML(job.id)}">Run this next</button>` : "") +
              (cancellable ? `<button type="button" data-optimization-cancel="${escapeHTML(job.id)}">Cancel job</button>` : "") +
              (retryable ? `<button type="button" data-optimization-retry="${escapeHTML(job.id)}">Retry</button>` : "") +
              `</article>`;
          }).join("");
        }
        if (jobs.some(job => ["queued", "copying-source", "encoding", "validating", "copying-result"].includes(job.status))) {
          optimizationPollTimer = setTimeout(refreshAudiobookJobs, 2000);
        } else if (optimizationPollTimer) {
          clearTimeout(optimizationPollTimer); optimizationPollTimer = null;
        }
      } catch {
        container.innerHTML = `<p role="alert">The audiobook job queue is unavailable. Check that FFmpeg and FFprobe are installed.</p>`;
      } finally {
        optimizationJobsRefreshInFlight = false;
      }
    }

    async function queueSelectedAudiobooks() {
      const itemIDs = [...selectedOptimizationItems];
      const approvedCatalogCoverIDs = [...approvedOptimizationCovers].filter(id => selectedOptimizationItems.has(id));
      const bitrateKbps = Object.fromEntries(itemIDs.map(id => [id,
        optimizationBitrates.get(id) || Number(optimizationPanel.querySelector(`[data-optimization-bitrate="${CSS.escape(id)}"]`)?.value || 0),
      ]));
      if (!itemIDs.length) {
        document.querySelector("#optimizationStatus").textContent = "Select at least one audiobook recommendation first.";
        return;
      }
      if (optimizationQueuePostInFlight) return;
      optimizationQueuePostInFlight = true;
      const queueButton = document.querySelector("#optimizationQueueSelected");
      queueButton.disabled = true;
      document.querySelector("#optimizationStatus").textContent = "Adding selected audiobooks to the staged conversion queue…";
      try {
        const response = await fetch(api("/api/optimization/audiobooks/jobs"), {
          method: "POST", headers: {"Content-Type":"application/json"},
          body: JSON.stringify({ itemIDs, approvedCatalogCoverIDs, bitrateKbps }),
    });
        const result = await response.json();
        if (!response.ok) throw new Error(result.error || "Could not queue selected audiobooks");
        for (const id of itemIDs) selectedOptimizationItems.delete(id);
        for (const id of approvedCatalogCoverIDs) approvedOptimizationCovers.delete(id);
        syncOptimizationSelectionState();
        document.querySelector("#optimizationStatus").textContent = `${result.jobs.length} audiobook job(s) started in sequence. Original files remain untouched.`;
        await refreshAudiobookJobs();
      } catch (err) {
        document.querySelector("#optimizationStatus").textContent = err.message;
      } finally {
        optimizationQueuePostInFlight = false;
        syncOptimizationSelectionState();
      }
    }

    async function controlAudiobookQueue() {
      const button = document.querySelector("#optimizationPause");
      const resume = button.dataset.queueAction === "resume";
      const endpoint = resume ? "resume" : "pause";
      try {
        const response = await fetch(api(`/api/optimization/audiobooks/queue/${endpoint}`), { method: "POST" });
        if (!response.ok) throw new Error("Queue control failed");
        document.querySelector("#optimizationStatus").textContent = resume ? "Queue resumed." : "Queue will pause after the active book finishes.";
        await refreshAudiobookJobs();
      } catch (err) { document.querySelector("#optimizationStatus").textContent = err.message; }
    }

    async function cancelAudiobookJob(id) {
      try {
        const response = await fetch(api(`/api/optimization/audiobooks/jobs/${encodeURIComponent(id)}`), { method: "DELETE" });
        if (!response.ok) throw new Error("Could not cancel job");
        document.querySelector("#optimizationStatus").textContent = "Job canceled; partial local output was discarded.";
        refreshAudiobookJobs();
      } catch (err) { document.querySelector("#optimizationStatus").textContent = err.message; }
    }

    async function prioritizeAudiobookJob(id) {
      try {
        const response = await fetch(api(`/api/optimization/audiobooks/jobs/${encodeURIComponent(id)}/prioritize`), { method: "POST" });
        if (!response.ok) throw new Error("Could not prioritize job");
        document.querySelector("#optimizationStatus").textContent = "Job moved to the front of the queue.";
        refreshAudiobookJobs();
      } catch (err) { document.querySelector("#optimizationStatus").textContent = err.message; }
    }

    async function promoteAudiobookJob(id) {
      try {
        const response = await fetch(api(`/api/optimization/audiobooks/jobs/${encodeURIComponent(id)}/promote`), { method: "POST" });
        if (!response.ok) { const body = await response.json().catch(() => ({})); throw new Error(body.error || "Could not install optimized file"); }
        document.querySelector("#optimizationStatus").textContent = "Optimized Opus file installed; original removed.";
        refreshAudiobookJobs();
      } catch (err) { document.querySelector("#optimizationStatus").textContent = err.message; }
    }

    async function retryAudiobookJob(id) {
      try {
        const response = await fetch(api(`/api/optimization/audiobooks/jobs/${encodeURIComponent(id)}/retry`), { method: "POST" });
        if (!response.ok) throw new Error("Could not retry job");
        document.querySelector("#optimizationStatus").textContent = "Job queued for a clean retry.";
        refreshAudiobookJobs();
      } catch (err) { document.querySelector("#optimizationStatus").textContent = err.message; }
    }

    async function reviewAudiobookJob(id, decision) {
      const note = decision === "accepted" ? "Listened to staged output and checked playback on a target device." : "Listening or playback needs a different encoding decision.";
      try {
        const response = await fetch(api(`/api/optimization/audiobooks/jobs/${encodeURIComponent(id)}/review`), {
          method: "POST", headers: {"Content-Type":"application/json"},
          body: JSON.stringify({ decision, note }),
        });
        if (!response.ok) throw new Error("Could not record playback review");
        document.querySelector("#optimizationStatus").textContent = decision === "accepted" ? "Playback review recorded." : "Output flagged for revision; original remains unchanged.";
        refreshAudiobookJobs();
      } catch (err) { document.querySelector("#optimizationStatus").textContent = err.message; }
    }

    async function enrichNow() {
      const r = await fetch(api("/api/settings/enrich"), { method:"POST" });
      if (r.status !== 202) return flashStatus("Enrichment already running");
      flashStatus("Fetching posters & summaries…");
      pollStatus(() => {
        refreshLibrary().then(() => {
          renderSettings();
          flashStatus("Enrichment done ✓");
        });
      });
    }

    function pollUntilScanned() {
      pollStatus(() => {
        refreshLibrary().then(() => flashStatus("Scan complete ✓"));
      });
    }

    async function putSettings(body) {
      const status = document.querySelector("#settingsStatus");
      status.textContent = "Saving…";
      body.version = settingsData?.version;
      try {
        const r = await fetch(api("/api/settings"), {
          method:"PUT", headers:{"Content-Type":"application/json"},
          body: JSON.stringify(body),
        });
        if (r.status === 409) {
          status.textContent = "Settings changed elsewhere — reloading latest";
          await loadSettings();
          setTimeout(() => status.textContent = "", 3500);
          return false;
        }
        settingsData = await r.json();
        await loadCacheStatus();
        applyLibraryLayout(settingsData.libraryLayout || "rails");
        hideEmptyLibraries = settingsData.hideEmptyLibraries !== false;
        applyEmptyLibraryTabs();
        applyTheme(settingsData.themePreset || "earthy");
        renderSettings();
        status.textContent = "Saved ✓";
        setTimeout(() => status.textContent = "", 2500);
        return true;
      } catch (e) {
        status.textContent = "Save failed";
        return false;
      }
    }

    function flashStatus(msg) {
      const el = document.querySelector("#settingsStatus");
      el.textContent = msg;
      setTimeout(() => el.textContent = "", 3000);
    }

    // ---------- Folder browser ----------
    async function browseTo(path) {
      const crumbs = document.querySelector("#browseCrumbs");
      const list = document.querySelector("#browserList");
      list.innerHTML = `<div style="padding:10px;color:var(--muted)">Loading…</div>`;
      try {
        const u = api("/api/settings/browse" + (path ? "?path=" + encodeURIComponent(path) : ""));
        const data = await (await fetch(u)).json();
        if (data.error) { list.innerHTML = `<div style="padding:10px">${escapeHTML(data.error)}</div>`; return; }
        browsedPath = data.path || "";
        renderCrumbs(data);
        document.querySelector("#selectedPath").textContent = browsedPath;
        if ((data.entries ?? []).length === 0) {
          list.innerHTML = `<div style="padding:10px;color:var(--muted)">No subfolders</div>`;
          return;
        }
        list.innerHTML = data.entries.map(e => `
          <button class="browse-row" data-path="${escapeHTML(e.path)}" data-action="browse">
            <span style="color:var(--accent);margin-right:8px">▸</span>${escapeHTML(e.name)}
          </button>`).join("");
      } catch (err) {
        list.innerHTML = `<div style="padding:10px">Browse failed</div>`;
      }
    }

    function renderCrumbs(data) {
      const el = document.querySelector("#browseCrumbs");
      if (!data.path) {
        el.innerHTML = `<span style="color:var(--muted)">Choose a starting point:</span>`;
        return;
      }
      const parts = data.path.split("/").filter(Boolean);
      let acc = "";
      let html = `<button class="crumb" data-path="/" data-action="browse">/</button> `;
      for (const seg of parts) {
        acc += "/" + seg;
        html += `<button class="crumb" data-path="${escapeHTML(acc)}" data-action="browse">${escapeHTML(seg)}</button> <span style="color:var(--muted)">/</span> `;
      }
      el.innerHTML = html;
    }

    function chooseBrowsed() {
      if (!browsedPath) return flashStatus("Navigate into a folder first");
      addLibraryRow();
    }
