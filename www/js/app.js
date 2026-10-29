// Dynamically determine the base URL from the current window location
const BASE_URL = window.location.origin;

document.getElementById('current-year').textContent = new Date().getFullYear();

let currentRecipeData = null;
let currentImageUrl = '';
let slugManuallyEdited = false;

document.addEventListener('click', (event) => {
    if (event.target.closest('[onclick], a[href], button[type="submit"]')) {
        window.scrollTo({ top: 0, behavior: 'smooth' });
    }
});

function showNotification(message, type = 'info') {
    const notificationStyles = {
        success: { icon: '✓', className: 'notification-success' },
        error: { icon: '!', className: 'notification-error' },
        info: { icon: 'i', className: 'notification-info' }
    };
    const style = notificationStyles[type] || notificationStyles.info;
    const notification = document.createElement('div');
    notification.className = `notification ${style.className}`;
    notification.setAttribute('role', type === 'error' ? 'alert' : 'status');

    const icon = document.createElement('span');
    icon.className = 'notification-icon';
    icon.setAttribute('aria-hidden', 'true');
    icon.textContent = style.icon;

    const messageElement = document.createElement('span');
    messageElement.className = 'notification-message';
    messageElement.textContent = message;

    const dismissButton = document.createElement('button');
    dismissButton.className = 'notification-dismiss';
    dismissButton.type = 'button';
    dismissButton.setAttribute('aria-label', 'Dismiss notification');
    dismissButton.textContent = '×';
    dismissButton.addEventListener('click', () => notification.remove());

    notification.append(icon, messageElement, dismissButton);
    document.getElementById('notifications').appendChild(notification);
    window.setTimeout(() => notification.remove(), 5000);
}

/**
 * Switch between the main navigation modes
 */
function switchMode(mode) {
    const viewBrowse = document.getElementById('view-browse');
    const viewDetail = document.getElementById('view-detail');
    const viewForm = document.getElementById('view-form');
    const btnBrowse = document.getElementById('btn-browse');
    const btnAdd = document.getElementById('btn-add');

    // Reset all views first
    viewBrowse.classList.add('hidden');
    viewDetail.classList.add('hidden');
    viewForm.classList.add('hidden');

    // Set active view
    if (mode === 'browse') {
        if (/^\/recipes\/[^/]+\/?$/.test(window.location.pathname)) {
            window.history.pushState(null, '', '/');
        }
        viewBrowse.classList.replace('hidden', 'block');
        document.getElementById('search-input').parentElement.classList.remove('hidden');
        btnBrowse.classList.replace('bg-zinc-800', 'bg-orange-600');
        btnAdd.classList.replace('bg-orange-600', 'bg-zinc-800');
        fetchRecipes();
    } else if (mode === 'form') {
        viewForm.classList.replace('hidden', 'block');
        btnBrowse.classList.replace('bg-orange-600', 'bg-zinc-800');
        btnAdd.classList.replace('bg-zinc-800', 'bg-orange-600');
        resetForm();
    }
}

function closeForm() {
    const wasEditing = Boolean(document.getElementById('recipe-id').value);
    resetForm();

    if (!wasEditing || !currentRecipeData) {
        switchMode('browse');
        return;
    }

    document.getElementById('view-form').classList.add('hidden');
    document.getElementById('view-detail').classList.remove('hidden');
    document.getElementById('search-input').parentElement.classList.add('hidden');
    document.getElementById('btn-browse').classList.replace('bg-zinc-800', 'bg-orange-600');
    document.getElementById('btn-add').classList.replace('bg-orange-600', 'bg-zinc-800');
}

/**
 * Fetch all recipes from the Go API
 */
async function fetchRecipes() {
    try {
        const res = await axios.get(`${BASE_URL}/api/recipes`);
        renderRecipes(res.data);
    } catch (err) {
        console.error("Failed to fetch recipes:", err);
    }
}

/**
 * Render the recipe gallery
 */
function renderRecipes(recipes) {
    const grid = document.getElementById('recipes-grid');
    grid.innerHTML = '';
    
    if (recipes.length === 0) {
        grid.innerHTML = `<div class="col-span-full py-20 text-center text-zinc-500">No recipes found.</div>`;
        return;
    }
    
    recipes.forEach(recipe => {
        const card = document.createElement('div');
        card.className = 'recipe-card bg-zinc-900 border border-zinc-800 rounded-2xl overflow-hidden p-4 flex flex-col';
        card.innerHTML = `
            <div class="relative mb-4 cursor-pointer" onclick="viewRecipe('${recipe.slug}')">
                <img src="${BASE_URL}${recipe.image_url}" 
                     class="w-full h-48 object-cover rounded-xl" 
                     onerror="this.onerror=null; this.src='/img/400x300-tava-no-image.png'">
                <span class="absolute top-2 right-2 bg-black/60 backdrop-blur-md px-2 py-1 rounded text-xs font-bold text-orange-400 border border-orange-500/30">
                    ${recipe.category}
                </span>
            </div>
            <h3 class="recipe-card-title text-xl font-bold mb-1">${recipe.title}</h3>
            <div class="text-sm text-zinc-400 mb-4 flex flex-wrap items-center gap-x-3 gap-y-1">
                <span>⏱ ${recipe.prep_time}</span>
                <span>🔥 ${recipe.cook_time}</span>
                <span>👥 ${recipe.servings}</span>
                <a href="${BASE_URL}/api/recipes/${encodeURIComponent(recipe.id)}/pdf"
                   download="${recipe.slug || 'recipe'}.pdf"
                   class="recipe-card-pdf-link"
                   aria-label="Download recipe as PDF"
                   title="Download recipe as PDF">📄︎</a>
            </div>
            <button onclick="viewRecipe('${recipe.slug}')" class="mt-auto w-full py-2 bg-zinc-800 rounded-lg hover:bg-zinc-700 transition text-orange-400 font-bold">View Details</button>
        `;
        grid.appendChild(card);
    });
}

/**
 * Show recipe details in the detail view container
 */
async function viewRecipe(slug, { replaceHistory = false, updateHistory = true } = {}) {
    try {
        const res = await axios.get(`${BASE_URL}/api/recipes/slug/${encodeURIComponent(slug)}`);
        currentRecipeData = res.data;

        const recipePath = `/recipes/${encodeURIComponent(currentRecipeData.slug)}`;
        if (updateHistory && window.location.pathname !== recipePath) {
            const updateState = replaceHistory ? 'replaceState' : 'pushState';
            window.history[updateState](null, '', recipePath);
        } else if (updateHistory && replaceHistory) {
            window.history.replaceState(null, '', recipePath);
        }

        // Transition to detail view
        document.getElementById('view-browse').classList.add('hidden');
        document.getElementById('view-form').classList.add('hidden');
        document.getElementById('view-detail').classList.remove('hidden');
        document.getElementById('search-input').parentElement.classList.add('hidden');
        document.getElementById('btn-browse').classList.replace('bg-zinc-800', 'bg-orange-600');
        document.getElementById('btn-add').classList.replace('bg-orange-600', 'bg-zinc-800');

        // Populate UI
        document.getElementById('detail-title').innerText = currentRecipeData.title;
        const pdfLink = document.getElementById('download-recipe-pdf');
        pdfLink.href = `${BASE_URL}/api/recipes/${encodeURIComponent(currentRecipeData.id)}/pdf`;
        pdfLink.download = `${currentRecipeData.slug || 'recipe'}.pdf`;
        document.getElementById('detail-category').innerText = `Category: ${currentRecipeData.category}`;
        document.getElementById('detail-prep').innerText = currentRecipeData.prep_time;
        document.getElementById('detail-cook').innerText = currentRecipeData.cook_time;
        document.getElementById('detail-servings').innerText = currentRecipeData.servings;
        const detailImage = document.getElementById('detail-image');
        const fallbackImage = '/img/400x300-tava-no-image.png';
        detailImage.onerror = () => {
            detailImage.onerror = null;
            detailImage.src = fallbackImage;
        };
        detailImage.src = currentRecipeData.image_url
            ? `${BASE_URL}${currentRecipeData.image_url}`
            : fallbackImage;
        
        const ingredientsList = document.getElementById('detail-ingredients');
        ingredientsList.innerHTML = '';
        currentRecipeData.ingredients.split('\n').forEach(item => {
            if (item.trim()) {
                const li = document.createElement('li');
                li.textContent = item.trim();
                ingredientsList.appendChild(li);
            }
        });

        document.getElementById('detail-steps').innerHTML = currentRecipeData.steps.split('\n').map(step => {
            return `<p class="border-b border-zinc-800 pb-2">${step}</p>`;
        }).join('');

    } catch (err) {
        console.error("Error fetching recipe details:", err);
        showNotification("Error fetching recipe details.", 'error');
    }
}

/**
 * Open the form view and pre-fill with the recipe data
 */
function openEditMode() {
    if (!currentRecipeData) return;

    switchMode('form');
    
    // Preserve the full path exactly as it is in the database
    currentImageUrl = currentRecipeData.image_url || '';

    document.getElementById('form-title').innerText = 'Edit Recipe';
    slugManuallyEdited = true;
    document.getElementById('recipe-id').value = currentRecipeData.id;
    document.getElementById('title').value = currentRecipeData.title;
    document.getElementById('slug').value = currentRecipeData.slug;
    document.getElementById('category').value = currentRecipeData.category;
    document.getElementById('prep_time').value = currentRecipeData.prep_time;
    document.getElementById('cook_time').value = currentRecipeData.cook_time;
    document.getElementById('servings').value = currentRecipeData.servings;
    document.getElementById('ingredients').value = currentRecipeData.ingredients;
    document.getElementById('steps').value = currentRecipeData.steps;
    
    // Update preview if an image was already associated
    if (currentImageUrl.includes('upload/')) {
        document.getElementById('preview').innerHTML = `<img src="${BASE_URL}${currentImageUrl}" class="h-full w-full object-cover">`;
    } else {
        document.getElementById('preview').innerHTML = '<span class="text-zinc-600 text-xs">No preview</span>';
    }
}

/**
 * Delete a recipe
 */
function deleteRecipe() {
    if (!currentRecipeData) return;

    document.getElementById('delete-dialog-message').textContent =
        `Are you sure you want to delete "${currentRecipeData.title}"? This action cannot be undone.`;
    document.getElementById('delete-confirmation').showModal();
}

async function confirmDeleteRecipe() {
    if (!currentRecipeData) return;

    const recipe = currentRecipeData;
    const confirmButton = document.getElementById('confirm-delete-button');
    confirmButton.disabled = true;
    confirmButton.textContent = 'Deleting...';

    try {
        await axios.delete(`${BASE_URL}/api/recipes/${recipe.id}`);
        document.getElementById('delete-confirmation').close();
        showNotification("Recipe deleted.", 'success');
        switchMode('browse');
    } catch (err) {
        console.error("Error deleting recipe:", err);
        document.getElementById('delete-confirmation').close();
        showNotification("Error deleting recipe.", 'error');
    } finally {
        confirmButton.disabled = false;
        confirmButton.textContent = 'Delete recipe';
    }
}

/**
 * Clear the form
 */
function resetForm() {
    document.getElementById('recipe-form').reset();
    document.getElementById('recipe-id').value = '';
    document.getElementById('form-title').innerText = 'Create New Recipe';
    document.getElementById('preview').innerHTML = '<span class="text-zinc-600 text-xs">No preview</span>';
    currentImageUrl = '';
    slugManuallyEdited = false;
}

/**
 * Handle Form Submission
 */
document.getElementById('recipe-form').onsubmit = async (e) => {
    e.preventDefault();
    const id = document.getElementById('recipe-id').value;
    
    let servingsVal = parseInt(document.getElementById('servings').value);
    if (isNaN(servingsVal)) {
        showNotification("Please enter a valid number for Servings.", 'error');
        return;
    }

    const recipeData = {
        title: document.getElementById('title').value,
        slug: document.getElementById('slug').value,
        category: document.getElementById('category').value,
        prep_time: document.getElementById('prep_time').value,
        cook_time: document.getElementById('cook_time').value,
        servings: servingsVal,
        ingredients: document.getElementById('ingredients').value,
        steps: document.getElementById('steps').value,
        image_url: currentImageUrl // This holds the original path if no new image is uploaded
    };

    try {
        const imgInput = document.getElementById('image-input');
        if (imgInput.files && imgInput.files[0]) {
            const formData = new FormData();
            formData.append('image', imgInput.files[0]);
            
            const uploadRes = await axios.post(`${BASE_URL}/api/upload`, formData, {
                headers: { 'Content-Type': 'multipart/form-data' }
            });
            
            // Update the image_url with the new full path returned by the server
            recipeData.image_url = uploadRes.data.url;
        }

        let url = id ? `${BASE_URL}/api/recipes/${id}` : `${BASE_URL}/api/recipes`;
        
        if (id) {
            recipeData.id = id;
            await axios({
                method: 'PUT',
                url: url,
                data: recipeData
            });
        } else {
            await axios.post(`${BASE_URL}/api/recipes`, recipeData);
        }

        showNotification('Recipe saved successfully!', 'success');
        if (id) {
            await viewRecipe(recipeData.slug, { replaceHistory: true });
        } else {
            switchMode('browse');
        }
    } catch (err) {
        console.error("Submission Error:", err);
        showNotification(`Error saving recipe: ${err.message || 'Check console for details'}`, 'error');
    }
};

/**
 * Image selection and drag-and-drop
 */
const imageInput = document.getElementById('image-input');
const imageDropZone = document.getElementById('image-drop-zone');

function showImagePreview(file) {
    if (!file.type.startsWith('image/')) {
        showNotification('Please choose an image file.', 'error');
        return;
    }

    const reader = new FileReader();
    reader.onload = function(event) {
        document.getElementById('preview').innerHTML = `<img src="${event.target.result}" class="h-full w-full object-cover" alt="Selected recipe image">`;
    };
    reader.onerror = function() {
        showNotification('Unable to preview the selected image.', 'error');
    };
    reader.readAsDataURL(file);
}

imageInput.onchange = function(event) {
    const file = event.target.files[0];
    if (file) showImagePreview(file);
};

imageDropZone.addEventListener('dragover', (event) => {
    event.preventDefault();
    imageDropZone.classList.add('is-dragging');
});

imageDropZone.addEventListener('dragleave', (event) => {
    if (!imageDropZone.contains(event.relatedTarget)) {
        imageDropZone.classList.remove('is-dragging');
    }
});

imageDropZone.addEventListener('drop', (event) => {
    event.preventDefault();
    imageDropZone.classList.remove('is-dragging');

    const file = event.dataTransfer.files[0];
    if (!file) return;
    if (!file.type.startsWith('image/')) {
        showNotification('Please choose an image file.', 'error');
        return;
    }

    const files = new DataTransfer();
    files.items.add(file);
    imageInput.files = files.files;
    showImagePreview(file);
});

document.getElementById('title').addEventListener('input', (event) => {
    if (document.getElementById('recipe-id').value || slugManuallyEdited) return;
    document.getElementById('slug').value = event.target.value
        .normalize('NFD')
        .replace(/[\u0300-\u036f]/g, '')
        .toLowerCase()
        .replace(/[^a-z0-9]+/g, '-')
        .replace(/^-|-$/g, '');
});

document.getElementById('slug').addEventListener('input', () => {
    slugManuallyEdited = true;
});

window.addEventListener('popstate', () => {
    const match = window.location.pathname.match(/^\/recipes\/([^/]+)\/?$/);
    if (match) {
        viewRecipe(decodeURIComponent(match[1]), { updateHistory: false });
    } else {
        switchMode('browse');
    }
});

/**
 * Search Logic
 */
function normalizeSearchText(value) {
    return value
        .normalize('NFD')
        .replace(/[\u0300-\u036f]/g, '')
        .toLowerCase()
        .replace(/đ/g, 'd');
}

document.getElementById('search-input').oninput = (e) => {
    const query = normalizeSearchText(e.target.value);
    axios.get(`${BASE_URL}/api/recipes`).then(res => {
        const filtered = res.data.filter(rcp =>
            normalizeSearchText(rcp.title).includes(query) ||
            normalizeSearchText(rcp.category).includes(query) ||
            normalizeSearchText(rcp.ingredients).includes(query)
        );
        renderRecipes(filtered);
    });
};

// Open a directly linked recipe or load the browse view.
const initialRecipeMatch = window.location.pathname.match(/^\/recipes\/([^/]+)\/?$/);
if (initialRecipeMatch) {
    viewRecipe(decodeURIComponent(initialRecipeMatch[1]), { updateHistory: false });
} else {
    fetchRecipes();
}